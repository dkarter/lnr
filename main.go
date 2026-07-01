package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/huh"
)

type LinearTicket struct {
	Title       string
	Description string
	Estimate    string
	Labels      []string
	TeamId      string
	AssigneeId  string
	StatusId    string
}

type CreatedIssue struct {
	Identifier string `json:"issueId"`
	BranchName string `json:"branchName"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

type Issue struct {
	Identifier string `json:"issueId"`
	BranchName string `json:"branchName"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

type UserSelections struct {
	TeamId     string   `json:"teamId"`
	AssigneeId string   `json:"assigneeId"`
	Labels     []string `json:"labels"`
	Estimate   string   `json:"estimate"`
	StatusId   string   `json:"statusId"`
}

type CacheEntry struct {
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

const noCacheExpiration time.Duration = 0
const userSelectionsCacheKey = "user-selections"
const userSelectionsConfigFile = "defaults.json"
const mcpAuthHeaderPrefix = "mcp:"
const oauthTokenCacheKey = "oauth-token"
const oauthTokenRefreshSkew = time.Minute
const defaultOAuthScopes = "read write"

var linearOAuthAuthorizeURL = "https://mcp.linear.app/authorize"
var linearOAuthRegistrationURL = "https://mcp.linear.app/register"
var linearOAuthResource = "https://mcp.linear.app/mcp"
var linearOAuthTokenURL = "https://mcp.linear.app/token"

type OAuthTokenCache struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	ClientID     string    `json:"client_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type OAuthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	Scope            string `json:"scope"`
	RefreshToken     string `json:"refresh_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type OAuthClientRegistrationResponse struct {
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type MCPResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type MCPPage[T any] struct {
	Teams       []T    `json:"teams"`
	Labels      []T    `json:"labels"`
	Users       []T    `json:"users"`
	Issues      []T    `json:"issues"`
	HasNextPage bool   `json:"hasNextPage"`
	Cursor      string `json:"cursor"`
}

type MCPIssue struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	GitBranchName string `json:"gitBranchName"`
}

func getCacheDir() string {
	if xdgCacheHome := os.Getenv("XDG_CACHE_HOME"); xdgCacheHome != "" {
		return filepath.Join(xdgCacheHome, "lnr")
	}

	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "lnr")
}

func getConfigDir() string {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "lnr")
	}

	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lnr")
}

func getConfigPath(filename string) string {
	configDir := getConfigDir()
	os.MkdirAll(configDir, 0755)
	return filepath.Join(configDir, filename)
}

func getCachePath(key string) string {
	cacheDir := getCacheDir()
	os.MkdirAll(cacheDir, 0755)
	return filepath.Join(cacheDir, key+".json")
}

func loadFromCache(key string, ttl time.Duration) (interface{}, bool) {
	cachePath := getCachePath(key)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if ttl > 0 && time.Since(entry.Timestamp) > ttl {
		return nil, false
	}

	return entry.Data, true
}

func loadTypedFromCache[T any](key string, ttl time.Duration) (T, bool) {
	var target T
	data, found := loadFromCache(key, ttl)
	if !found {
		return target, false
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return target, false
	}

	if err := json.Unmarshal(jsonData, &target); err != nil {
		return target, false
	}

	return target, true
}

func saveToCache(key string, data interface{}) error {
	cachePath := getCachePath(key)
	entry := CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, jsonData, 0644)
}

func clearCache() error {
	cacheDir := getCacheDir()
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil // Cache directory doesn't exist, nothing to clear
	}
	return os.RemoveAll(cacheDir)
}

func clearConfig() error {
	configDir := getConfigDir()
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(configDir)
}

func resetData() error {
	if err := clearCache(); err != nil {
		return err
	}

	return clearConfig()
}

func getLinearAuthHeader() string {
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey != "" {
		return apiKey
	}

	accessToken := os.Getenv("LINEAR_OAUTH_ACCESS_TOKEN")
	if accessToken != "" {
		return bearerAuthHeader(accessToken)
	}

	scopes := oauthScopes()

	if cache, found := loadOAuthTokenCache(scopes); found {
		if cache.ExpiresAt.After(time.Now().Add(oauthTokenRefreshSkew)) {
			return mcpAuthHeader(cache.AccessToken)
		}

		if cache.RefreshToken != "" && cache.ClientID != "" {
			token, err := refreshOAuthAccessToken(cache.ClientID, cache.RefreshToken, scopes)
			if err == nil {
				if err := saveOAuthToken(cache.ClientID, scopes, token, cache.RefreshToken); err == nil {
					return mcpAuthHeader(token.AccessToken)
				}
			}
			fmt.Println("Cached Linear OAuth token expired; starting a new browser login.")
		}
	}

	token, err := runDCRLogin(scopes)
	if err != nil {
		fmt.Printf("❌ Error signing in to Linear: %v\n", err)
		fmt.Println("\nYou can still use a personal API key instead:")
		fmt.Println("  export LINEAR_API_KEY='your-api-key'")
		os.Exit(1)
	}

	return mcpAuthHeader(token.AccessToken)
}

func oauthScopes() string {
	scopes := os.Getenv("LINEAR_OAUTH_SCOPES")
	if scopes == "" {
		return defaultOAuthScopes
	}

	return scopes
}

func bearerAuthHeader(token string) string {
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}

	return "Bearer " + token
}

func mcpAuthHeader(token string) string {
	return mcpAuthHeaderPrefix + bearerAuthHeader(token)
}

func splitMCPAuthHeader(authHeader string) (string, bool) {
	if !strings.HasPrefix(authHeader, mcpAuthHeaderPrefix) {
		return authHeader, false
	}

	return strings.TrimPrefix(authHeader, mcpAuthHeaderPrefix), true
}

func loadOAuthTokenCache(scopes string) (OAuthTokenCache, bool) {
	data, err := os.ReadFile(getCachePath(oauthTokenCacheKey))
	if err != nil {
		return OAuthTokenCache{}, false
	}

	var cache OAuthTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return OAuthTokenCache{}, false
	}

	if cache.Scope != scopes || cache.AccessToken == "" {
		return OAuthTokenCache{}, false
	}

	return cache, true
}

func saveOAuthToken(clientID, scopes string, token OAuthTokenResponse, fallbackRefreshToken string) error {
	refreshToken := token.RefreshToken
	if refreshToken == "" {
		refreshToken = fallbackRefreshToken
	}

	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64((24 * time.Hour).Seconds())
	}

	return saveOAuthTokenCache(OAuthTokenCache{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: refreshToken,
		Scope:        scopes,
		ClientID:     clientID,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	})
}

func saveOAuthTokenCache(cache OAuthTokenCache) error {
	jsonData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	cachePath := getCachePath(oauthTokenCacheKey)
	if err := os.WriteFile(cachePath, jsonData, 0600); err != nil {
		return err
	}
	return os.Chmod(cachePath, 0600)
}

func clearOAuthTokenCache() error {
	err := os.Remove(getCachePath(oauthTokenCacheKey))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

type oauthCallbackResult struct {
	code string
	err  error
}

func runDCRLogin(scopes string) (OAuthTokenResponse, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return OAuthTokenResponse{}, err
	}

	callbackURL := fmt.Sprintf("http://%s/oauth/callback", listener.Addr().String())
	client, err := registerOAuthClient(callbackURL, scopes)
	if err != nil {
		listener.Close()
		return OAuthTokenResponse{}, err
	}

	state, err := randomURLSafeString(32)
	if err != nil {
		listener.Close()
		return OAuthTokenResponse{}, err
	}
	codeVerifier, err := randomURLSafeString(64)
	if err != nil {
		listener.Close()
		return OAuthTokenResponse{}, err
	}

	resultCh := make(chan oauthCallbackResult, 1)
	server := &http.Server{Handler: oauthCallbackHandler(state, resultCh)}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case resultCh <- oauthCallbackResult{err: err}:
			default:
			}
		}
	}()

	authURL, err := buildAuthorizationURL(client.ClientID, callbackURL, scopes, state, codeVerifier)
	if err != nil {
		shutdownOAuthServer(server)
		return OAuthTokenResponse{}, err
	}

	fmt.Println("Opening Linear sign-in in your browser...")
	fmt.Println(authURL)
	if err := openURL(authURL); err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
		fmt.Println("Open the URL above to continue.")
	}

	var result oauthCallbackResult
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Minute):
		shutdownOAuthServer(server)
		return OAuthTokenResponse{}, fmt.Errorf("timed out waiting for OAuth callback")
	}
	shutdownOAuthServer(server)

	if result.err != nil {
		return OAuthTokenResponse{}, result.err
	}

	token, err := exchangeOAuthCode(client.ClientID, result.code, callbackURL, codeVerifier, scopes)
	if err != nil {
		return OAuthTokenResponse{}, err
	}

	if err := saveOAuthToken(client.ClientID, scopes, token, ""); err != nil {
		return OAuthTokenResponse{}, err
	}

	return token, nil
}

func oauthCallbackHandler(expectedState string, resultCh chan<- oauthCallbackResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/callback" {
			http.NotFound(w, r)
			return
		}

		query := r.URL.Query()
		if errorCode := query.Get("error"); errorCode != "" {
			description := query.Get("error_description")
			if description == "" {
				description = errorCode
			}
			sendOAuthCallbackResult(resultCh, oauthCallbackResult{err: fmt.Errorf("OAuth authorization failed: %s", description)})
			http.Error(w, "Linear authorization failed. You can close this tab.", http.StatusBadRequest)
			return
		}

		if query.Get("state") != expectedState {
			sendOAuthCallbackResult(resultCh, oauthCallbackResult{err: fmt.Errorf("OAuth state mismatch")})
			http.Error(w, "OAuth state mismatch. You can close this tab.", http.StatusBadRequest)
			return
		}

		code := query.Get("code")
		if code == "" {
			sendOAuthCallbackResult(resultCh, oauthCallbackResult{err: fmt.Errorf("OAuth callback did not include a code")})
			http.Error(w, "OAuth callback did not include a code. You can close this tab.", http.StatusBadRequest)
			return
		}

		sendOAuthCallbackResult(resultCh, oauthCallbackResult{code: code})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h1>Linear sign-in complete</h1><p>You can close this tab and return to lnr.</p></body></html>")
	})
}

func sendOAuthCallbackResult(resultCh chan<- oauthCallbackResult, result oauthCallbackResult) {
	select {
	case resultCh <- result:
	default:
	}
}

func shutdownOAuthServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func registerOAuthClient(callbackURL, scopes string) (OAuthClientRegistrationResponse, error) {
	payload := map[string]interface{}{
		"client_name":                "lnr",
		"client_uri":                 "https://github.com/dkarter/lnr",
		"redirect_uris":              []string{callbackURL},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"scope":                      scopes,
		"token_endpoint_auth_method": "none",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return OAuthClientRegistrationResponse{}, err
	}

	req, err := http.NewRequest("POST", linearOAuthRegistrationURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return OAuthClientRegistrationResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return OAuthClientRegistrationResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuthClientRegistrationResponse{}, err
	}

	var client OAuthClientRegistrationResponse
	if len(body) > 0 {
		_ = json.Unmarshal(body, &client)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OAuthClientRegistrationResponse{}, oauthResponseError("OAuth client registration failed", body, client.Error, client.ErrorDescription)
	}
	if client.Error != "" {
		return OAuthClientRegistrationResponse{}, oauthResponseError("OAuth client registration failed", body, client.Error, client.ErrorDescription)
	}
	if client.ClientID == "" {
		return OAuthClientRegistrationResponse{}, fmt.Errorf("OAuth client registration response did not include client_id")
	}

	return client, nil
}

func buildAuthorizationURL(clientID, callbackURL, scopes, state, codeVerifier string) (string, error) {
	authorizeURL, err := url.Parse(linearOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}

	query := authorizeURL.Query()
	query.Set("client_id", clientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("response_type", "code")
	query.Set("scope", scopes)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge(codeVerifier))
	query.Set("code_challenge_method", "S256")
	if linearOAuthResource != "" {
		query.Set("resource", linearOAuthResource)
	}
	authorizeURL.RawQuery = query.Encode()

	return authorizeURL.String(), nil
}

func exchangeOAuthCode(clientID, code, callbackURL, codeVerifier, scopes string) (OAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", callbackURL)
	form.Set("client_id", clientID)
	form.Set("code_verifier", codeVerifier)
	form.Set("scope", scopes)
	if linearOAuthResource != "" {
		form.Set("resource", linearOAuthResource)
	}

	return fetchOAuthAccessToken(form)
}

func refreshOAuthAccessToken(clientID, refreshToken, scopes string) (OAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("scope", scopes)
	if linearOAuthResource != "" {
		form.Set("resource", linearOAuthResource)
	}

	return fetchOAuthAccessToken(form)
}

func fetchOAuthAccessToken(form url.Values) (OAuthTokenResponse, error) {
	req, err := http.NewRequest("POST", linearOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuthTokenResponse{}, err
	}

	var token OAuthTokenResponse
	if len(body) > 0 {
		_ = json.Unmarshal(body, &token)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OAuthTokenResponse{}, oauthResponseError("OAuth token request failed", body, token.Error, token.ErrorDescription)
	}
	if token.Error != "" {
		return OAuthTokenResponse{}, oauthResponseError("OAuth token request failed", body, token.Error, token.ErrorDescription)
	}
	if token.AccessToken == "" {
		return OAuthTokenResponse{}, fmt.Errorf("OAuth token response did not include access_token")
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}

	return token, nil
}

func oauthResponseError(prefix string, body []byte, errorCode, description string) error {
	if description != "" {
		return fmt.Errorf("%s: %s", prefix, description)
	}
	if errorCode != "" {
		return fmt.Errorf("%s: %s", prefix, errorCode)
	}
	return fmt.Errorf("%s: %s", prefix, strings.TrimSpace(string(body)))
}

func randomURLSafeString(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return cmd.Run()
}

func callMCPTool(authHeader, name string, arguments map[string]interface{}) ([]byte, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": arguments,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", linearOAuthResource, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Linear MCP error: %s", strings.TrimSpace(string(body)))
	}

	data, err := extractSSEData(body)
	if err != nil {
		return nil, err
	}

	var mcpResponse MCPResponse
	if err := json.Unmarshal(data, &mcpResponse); err != nil {
		return nil, err
	}
	if mcpResponse.Error != nil {
		return nil, fmt.Errorf("Linear MCP error: %s", mcpResponse.Error.Message)
	}

	for _, content := range mcpResponse.Result.Content {
		if content.Type == "text" && content.Text != "" {
			return []byte(content.Text), nil
		}
	}

	return nil, fmt.Errorf("Linear MCP response did not include text content")
}

func extractSSEData(body []byte) ([]byte, error) {
	text := string(body)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty Linear MCP response")
	}

	var dataLines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if len(dataLines) == 0 {
		return body, nil
	}

	return []byte(strings.Join(dataLines, "\n")), nil
}

func fetchMCPTeams(authHeader string) ([]Team, error) {
	var teamList []Team
	var cursor string
	for {
		arguments := map[string]interface{}{"limit": 250}
		if cursor != "" {
			arguments["cursor"] = cursor
		}

		data, err := callMCPTool(authHeader, "list_teams", arguments)
		if err != nil {
			return nil, err
		}

		var page MCPPage[Team]
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		teamList = append(teamList, page.Teams...)
		if !page.HasNextPage || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}

	return teamList, nil
}

func fetchMCPTeamLabels(authHeader, teamID string) ([]Label, error) {
	var labelList []Label
	var cursor string
	for {
		arguments := map[string]interface{}{"team": teamID, "limit": 250}
		if cursor != "" {
			arguments["cursor"] = cursor
		}

		data, err := callMCPTool(authHeader, "list_issue_labels", arguments)
		if err != nil {
			return nil, err
		}

		var page MCPPage[Label]
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		labelList = append(labelList, page.Labels...)
		if !page.HasNextPage || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}

	return labelList, nil
}

func fetchMCPTeamUsers(authHeader, teamID string) ([]User, error) {
	var userList []User
	var cursor string
	for {
		arguments := map[string]interface{}{"team": teamID, "limit": 250}
		if cursor != "" {
			arguments["cursor"] = cursor
		}

		data, err := callMCPTool(authHeader, "list_users", arguments)
		if err != nil {
			return nil, err
		}

		var page MCPPage[User]
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		userList = append(userList, page.Users...)
		if !page.HasNextPage || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}

	return userList, nil
}

func fetchMCPWorkflowStates(authHeader, teamID string) ([]WorkflowState, error) {
	data, err := callMCPTool(authHeader, "list_issue_statuses", map[string]interface{}{"team": teamID})
	if err != nil {
		return nil, err
	}

	var states []WorkflowState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, err
	}

	return states, nil
}

func fetchMCPTeamIssues(authHeader, teamID string) ([]Issue, error) {
	var issueList []Issue
	var cursor string
	for {
		arguments := map[string]interface{}{"team": teamID, "limit": 250}
		if cursor != "" {
			arguments["cursor"] = cursor
		}

		data, err := callMCPTool(authHeader, "list_issues", arguments)
		if err != nil {
			return nil, err
		}

		var page MCPPage[MCPIssue]
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		for _, issue := range page.Issues {
			issueList = append(issueList, Issue{
				Identifier: issue.ID,
				BranchName: issue.GitBranchName,
				Title:      issue.Title,
				URL:        issue.URL,
			})
		}
		if !page.HasNextPage || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}

	return issueList, nil
}

func createLinearTicketWithMCP(authHeader string, ticket LinearTicket) (CreatedIssue, error) {
	arguments := map[string]interface{}{
		"title": ticket.Title,
		"team":  ticket.TeamId,
	}
	if ticket.Description != "" {
		arguments["description"] = ticket.Description
	}
	if ticket.Estimate != "" && ticket.Estimate != "0" {
		if estimate, err := strconv.Atoi(ticket.Estimate); err == nil {
			arguments["estimate"] = estimate
		}
	}
	if len(ticket.Labels) > 0 {
		arguments["labels"] = ticket.Labels
	}
	if ticket.AssigneeId != "" {
		arguments["assignee"] = ticket.AssigneeId
	}
	if ticket.StatusId != "" {
		arguments["state"] = ticket.StatusId
	}

	data, err := callMCPTool(authHeader, "save_issue", arguments)
	if err != nil {
		return CreatedIssue{}, err
	}

	var issue MCPIssue
	if err := json.Unmarshal(data, &issue); err != nil {
		return CreatedIssue{}, err
	}
	if issue.ID == "" {
		return CreatedIssue{}, fmt.Errorf("Linear MCP response did not include issue id")
	}

	return CreatedIssue{
		Identifier: issue.ID,
		BranchName: issue.GitBranchName,
		Title:      issue.Title,
		URL:        issue.URL,
	}, nil
}

func loadUserSelections() UserSelections {
	configPath := getConfigPath(userSelectionsConfigFile)
	data, err := os.ReadFile(configPath)
	if err == nil {
		var selections UserSelections
		if err := json.Unmarshal(data, &selections); err == nil {
			return selections
		}
	}

	if selections, found := loadTypedFromCache[UserSelections](userSelectionsCacheKey, noCacheExpiration); found {
		_ = saveUserSelections(selections)
		return selections
	}

	return UserSelections{}
}

func saveUserSelections(selections UserSelections) error {
	jsonData, err := json.MarshalIndent(selections, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getConfigPath(userSelectionsConfigFile), jsonData, 0644)
}

func fallbackBranchName(issue CreatedIssue) string {
	if issue.BranchName != "" {
		return issue.BranchName
	}

	return strings.ToLower(issue.Identifier)
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func makeLinearRequest(apiKey, query string, variables map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		return nil, fmt.Errorf("Linear API error: %v", errors)
	}

	return result, nil
}

func fetchTeamLabels(apiKey, teamId string) ([]Label, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return fetchMCPTeamLabels(authHeader, teamId)
	}

	var labelList []Label
	var after string

	for {
		query := `
			query TeamLabels($teamId: String!, $after: String) {
				team(id: $teamId) {
					labels(first: 50, after: $after) {
						nodes {
							id
							name
						}
						pageInfo {
							hasNextPage
							endCursor
						}
					}
				}
			}
		`

		variables := map[string]interface{}{"teamId": teamId}
		if after != "" {
			variables["after"] = after
		}

		result, err := makeLinearRequest(apiKey, query, variables)
		if err != nil {
			return nil, err
		}

		data := result["data"].(map[string]interface{})
		team := data["team"].(map[string]interface{})
		labels := team["labels"].(map[string]interface{})
		nodes := labels["nodes"].([]interface{})
		pageInfo := labels["pageInfo"].(map[string]interface{})

		for _, node := range nodes {
			label := node.(map[string]interface{})
			labelList = append(labelList, Label{
				ID:   label["id"].(string),
				Name: label["name"].(string),
			})
		}

		hasNextPage := pageInfo["hasNextPage"].(bool)
		if !hasNextPage {
			break
		}

		if endCursor, ok := pageInfo["endCursor"].(string); ok {
			after = endCursor
		} else {
			break
		}
	}

	return labelList, nil
}

func fetchTeams(apiKey string) ([]Team, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return fetchMCPTeams(authHeader)
	}

	var teamList []Team
	var after string

	for {
		query := `
			query Teams($after: String) {
				teams(first: 50, after: $after) {
					nodes {
						id
						name
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		`

		variables := map[string]interface{}{}
		if after != "" {
			variables["after"] = after
		}

		result, err := makeLinearRequest(apiKey, query, variables)
		if err != nil {
			return nil, err
		}

		data := result["data"].(map[string]interface{})
		teams := data["teams"].(map[string]interface{})
		nodes := teams["nodes"].([]interface{})
		pageInfo := teams["pageInfo"].(map[string]interface{})

		for _, node := range nodes {
			team := node.(map[string]interface{})
			teamList = append(teamList, Team{
				ID:   team["id"].(string),
				Name: team["name"].(string),
			})
		}

		hasNextPage := pageInfo["hasNextPage"].(bool)
		if !hasNextPage {
			break
		}

		if endCursor, ok := pageInfo["endCursor"].(string); ok {
			after = endCursor
		} else {
			break
		}
	}

	return teamList, nil
}

func fetchTeamInfo(apiKey, teamId string) (*Team, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		teams, err := fetchMCPTeams(authHeader)
		if err != nil {
			return nil, err
		}
		for _, team := range teams {
			if team.ID == teamId {
				return &team, nil
			}
		}
		return nil, fmt.Errorf("team not found: %s", teamId)
	}

	query := `
		query Team($teamId: String!) {
			team(id: $teamId) {
				id
				name
			}
		}
	`

	result, err := makeLinearRequest(apiKey, query, map[string]interface{}{"teamId": teamId})
	if err != nil {
		return nil, err
	}

	data := result["data"].(map[string]interface{})
	team := data["team"].(map[string]interface{})

	return &Team{
		ID:   team["id"].(string),
		Name: team["name"].(string),
	}, nil
}

func fetchTeamUsers(apiKey, teamId string) ([]User, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return fetchMCPTeamUsers(authHeader, teamId)
	}

	var userList []User
	var after string

	for {
		query := `
			query TeamUsers($teamId: String!, $after: String) {
				team(id: $teamId) {
					organization {
						users(first: 50, after: $after) {
							nodes {
								id
								name
								email
							}
							pageInfo {
								hasNextPage
								endCursor
							}
						}
					}
				}
			}
		`

		variables := map[string]interface{}{"teamId": teamId}
		if after != "" {
			variables["after"] = after
		}

		result, err := makeLinearRequest(apiKey, query, variables)
		if err != nil {
			return nil, err
		}

		data := result["data"].(map[string]interface{})
		team := data["team"].(map[string]interface{})
		org := team["organization"].(map[string]interface{})
		users := org["users"].(map[string]interface{})
		nodes := users["nodes"].([]interface{})
		pageInfo := users["pageInfo"].(map[string]interface{})

		for _, node := range nodes {
			user := node.(map[string]interface{})
			userList = append(userList, User{
				ID:    user["id"].(string),
				Name:  user["name"].(string),
				Email: user["email"].(string),
			})
		}

		hasNextPage := pageInfo["hasNextPage"].(bool)
		if !hasNextPage {
			break
		}

		if endCursor, ok := pageInfo["endCursor"].(string); ok {
			after = endCursor
		} else {
			break
		}
	}

	return userList, nil
}

func fetchWorkflowStates(apiKey, teamId string) ([]WorkflowState, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return fetchMCPWorkflowStates(authHeader, teamId)
	}

	var stateList []WorkflowState
	var after string

	for {
		query := `
			query TeamWorkflowStates($teamId: String!, $after: String) {
				team(id: $teamId) {
					states(first: 50, after: $after) {
						nodes {
							id
							name
							type
						}
						pageInfo {
							hasNextPage
							endCursor
						}
					}
				}
			}
		`

		variables := map[string]interface{}{"teamId": teamId}
		if after != "" {
			variables["after"] = after
		}

		result, err := makeLinearRequest(apiKey, query, variables)
		if err != nil {
			return nil, err
		}

		data := result["data"].(map[string]interface{})
		team := data["team"].(map[string]interface{})
		states := team["states"].(map[string]interface{})
		nodes := states["nodes"].([]interface{})
		pageInfo := states["pageInfo"].(map[string]interface{})

		for _, node := range nodes {
			state := node.(map[string]interface{})
			stateList = append(stateList, WorkflowState{
				ID:   state["id"].(string),
				Name: state["name"].(string),
				Type: state["type"].(string),
			})
		}

		hasNextPage := pageInfo["hasNextPage"].(bool)
		if !hasNextPage {
			break
		}

		if endCursor, ok := pageInfo["endCursor"].(string); ok {
			after = endCursor
		} else {
			break
		}
	}

	return stateList, nil
}

func loadTeams(apiKey string) ([]Team, error) {
	if teams, found := loadTypedFromCache[[]Team]("teams", noCacheExpiration); found {
		return teams, nil
	}

	teams, err := fetchTeams(apiKey)
	if err != nil {
		return nil, err
	}
	saveToCache("teams", teams)

	return teams, nil
}

func loadTeamLabels(apiKey, teamId string) ([]Label, error) {
	if labels, found := loadTypedFromCache[[]Label]("labels-"+teamId, noCacheExpiration); found {
		return labels, nil
	}

	labels, err := fetchTeamLabels(apiKey, teamId)
	if err != nil {
		return nil, err
	}
	saveToCache("labels-"+teamId, labels)

	return labels, nil
}

func loadTeamUsers(apiKey, teamId string) ([]User, error) {
	if users, found := loadTypedFromCache[[]User]("users-"+teamId, noCacheExpiration); found {
		return users, nil
	}

	users, err := fetchTeamUsers(apiKey, teamId)
	if err != nil {
		return nil, err
	}
	saveToCache("users-"+teamId, users)

	return users, nil
}

func loadWorkflowStates(apiKey, teamId string) ([]WorkflowState, error) {
	if states, found := loadTypedFromCache[[]WorkflowState]("states-"+teamId, noCacheExpiration); found {
		return states, nil
	}

	states, err := fetchWorkflowStates(apiKey, teamId)
	if err != nil {
		return nil, err
	}
	saveToCache("states-"+teamId, states)

	return states, nil
}

func fetchTeamIssues(apiKey, teamId string) ([]Issue, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return fetchMCPTeamIssues(authHeader, teamId)
	}

	var issues []Issue
	var after string

	for len(issues) < 250 {
		query := `
			query TeamIssues($teamId: String!, $after: String) {
				team(id: $teamId) {
					issues(first: 50, after: $after, orderBy: updatedAt) {
						nodes {
							identifier
							title
							branchName
							url
						}
						pageInfo {
							hasNextPage
							endCursor
						}
					}
				}
			}
		`

		variables := map[string]interface{}{"teamId": teamId}
		if after != "" {
			variables["after"] = after
		}

		result, err := makeLinearRequest(apiKey, query, variables)
		if err != nil {
			return nil, err
		}

		data := result["data"].(map[string]interface{})
		team := data["team"].(map[string]interface{})
		issueConnection := team["issues"].(map[string]interface{})
		nodes := issueConnection["nodes"].([]interface{})
		pageInfo := issueConnection["pageInfo"].(map[string]interface{})

		for _, node := range nodes {
			issue := node.(map[string]interface{})
			issues = append(issues, Issue{
				Identifier: issue["identifier"].(string),
				Title:      issue["title"].(string),
				BranchName: getString(issue, "branchName"),
				URL:        issue["url"].(string),
			})
		}

		if hasNextPage := pageInfo["hasNextPage"].(bool); !hasNextPage {
			break
		}

		if endCursor, ok := pageInfo["endCursor"].(string); ok {
			after = endCursor
		} else {
			break
		}
	}

	return issues, nil
}

func getEstimateOptions(estimateType int) []huh.Option[string] {
	switch estimateType {
	case 0: // No estimates
		return []huh.Option[string]{
			{Key: "No estimate", Value: "0"},
		}
	case 1: // T-shirt sizes
		return []huh.Option[string]{
			{Key: "XS - Extra Small", Value: "1"},
			{Key: "S - Small", Value: "2"},
			{Key: "M - Medium", Value: "3"},
			{Key: "L - Large", Value: "5"},
			{Key: "XL - Extra Large", Value: "8"},
		}
	case 2: // Fibonacci
		return []huh.Option[string]{
			{Key: "1", Value: "1"},
			{Key: "2", Value: "2"},
			{Key: "3", Value: "3"},
			{Key: "5", Value: "5"},
			{Key: "8", Value: "8"},
			{Key: "13", Value: "13"},
			{Key: "21", Value: "21"},
		}
	default: // Linear's default (story points)
		return []huh.Option[string]{
			{Key: "0 - No estimate", Value: "0"},
			{Key: "1 - Small (< 1 day)", Value: "1"},
			{Key: "2 - Medium (1-2 days)", Value: "2"},
			{Key: "3 - Large (3-5 days)", Value: "3"},
			{Key: "5 - Extra Large (1+ weeks)", Value: "5"},
			{Key: "8 - Epic (2+ weeks)", Value: "8"},
		}
	}
}

func teamOptions(teams []Team) []huh.Option[string] {
	options := make([]huh.Option[string], len(teams))
	for i, team := range teams {
		options[i] = huh.Option[string]{Key: team.Name, Value: team.ID}
	}

	return options
}

func labelOptions(labels []Label) ([]huh.Option[string], map[string]string) {
	options := make([]huh.Option[string], len(labels))
	labelMap := make(map[string]string)
	for i, label := range labels {
		options[i] = huh.Option[string]{Key: label.Name, Value: label.Name}
		labelMap[label.Name] = label.ID
	}

	return options, labelMap
}

func findTeam(teams []Team, teamId string) *Team {
	for _, team := range teams {
		if team.ID == teamId {
			return &team
		}
	}

	return nil
}

func requireDefaultTeam(selections UserSelections) string {
	if selections.TeamId == "" {
		fmt.Println("❌ No default team set")
		fmt.Println("Run `lnr set-team` first")
		os.Exit(1)
	}

	return selections.TeamId
}

func runSetTeam(apiKey string) {
	teams, err := loadTeams(apiKey)
	if err != nil {
		fmt.Printf("❌ Error fetching teams: %v\n", err)
		os.Exit(1)
	}

	selections := loadUserSelections()
	selectedTeamId := selections.TeamId
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default Team").
				Description("Filter and select the team to use for quick actions").
				Options(teamOptions(teams)...).
				Filtering(true).
				Value(&selectedTeamId),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Println("Team selection cancelled or error:", err)
		os.Exit(1)
	}

	if selections.TeamId != selectedTeamId {
		selections.AssigneeId = ""
		selections.Labels = nil
		selections.StatusId = ""
	}
	selections.TeamId = selectedTeamId
	if err := saveUserSelections(selections); err != nil {
		fmt.Printf("❌ Error saving default team: %v\n", err)
		os.Exit(1)
	}

	selectedTeam := findTeam(teams, selectedTeamId)
	if selectedTeam != nil {
		fmt.Printf("✅ Default team set to %s\n", selectedTeam.Name)
		return
	}
	fmt.Println("✅ Default team saved")
}

func runSetLabels(apiKey string) {
	selections := loadUserSelections()
	teamId := requireDefaultTeam(selections)

	labels, err := loadTeamLabels(apiKey, teamId)
	if err != nil {
		fmt.Printf("❌ Error fetching labels: %v\n", err)
		os.Exit(1)
	}

	selectedLabels := selections.Labels
	options, _ := labelOptions(labels)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Default Labels").
				Description("Filter and select labels to apply in quick mode").
				Options(options...).
				Filtering(true).
				Value(&selectedLabels).
				Limit(4),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Println("Label selection cancelled or error:", err)
		os.Exit(1)
	}

	selections.Labels = selectedLabels
	if err := saveUserSelections(selections); err != nil {
		fmt.Printf("❌ Error saving default labels: %v\n", err)
		os.Exit(1)
	}

	if len(selectedLabels) == 0 {
		fmt.Println("✅ Default labels cleared")
		return
	}
	fmt.Printf("✅ Default labels set to %s\n", strings.Join(selectedLabels, ", "))
}

func runSetEstimate() {
	selections := loadUserSelections()
	selectedEstimate := selections.Estimate
	estimateOptions := getEstimateOptions(1)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default Estimate").
				Description("Select the estimate to apply in quick mode").
				Options(estimateOptions...).
				Value(&selectedEstimate),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Println("Estimate selection cancelled or error:", err)
		os.Exit(1)
	}

	selections.Estimate = selectedEstimate
	if err := saveUserSelections(selections); err != nil {
		fmt.Printf("❌ Error saving default estimate: %v\n", err)
		os.Exit(1)
	}

	for _, option := range estimateOptions {
		if option.Value == selectedEstimate {
			fmt.Printf("✅ Default estimate set to %s\n", option.Key)
			return
		}
	}
	fmt.Println("✅ Default estimate saved")
}

func runSetStatus(apiKey string) {
	selections := loadUserSelections()
	teamId := requireDefaultTeam(selections)

	workflowStates, err := loadWorkflowStates(apiKey, teamId)
	if err != nil {
		fmt.Printf("❌ Error fetching workflow states: %v\n", err)
		os.Exit(1)
	}

	statusOptions := make([]huh.Option[string], len(workflowStates)+1)
	statusOptions[0] = huh.Option[string]{Key: "No default status", Value: ""}
	for i, state := range workflowStates {
		statusOptions[i+1] = huh.Option[string]{Key: state.Name, Value: state.ID}
	}

	selectedStatusId := selections.StatusId
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default Status").
				Description("Select the status to apply to new issues").
				Options(statusOptions...).
				Filtering(true).
				Value(&selectedStatusId),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Println("Status selection cancelled or error:", err)
		os.Exit(1)
	}

	selections.StatusId = selectedStatusId
	if err := saveUserSelections(selections); err != nil {
		fmt.Printf("❌ Error saving default status: %v\n", err)
		os.Exit(1)
	}

	if selectedStatusId == "" {
		fmt.Println("✅ Default status cleared")
		return
	}

	for _, state := range workflowStates {
		if state.ID == selectedStatusId {
			fmt.Printf("✅ Default status set to %s\n", state.Name)
			return
		}
	}
	fmt.Println("✅ Default status saved")
}

func runQuickCreate(apiKey, title string, jsonOutput bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		fmt.Println("❌ Title cannot be empty")
		os.Exit(1)
	}

	selections := loadUserSelections()
	teamId := requireDefaultTeam(selections)
	labels, err := loadTeamLabels(apiKey, teamId)
	if err != nil {
		fmt.Printf("❌ Error fetching labels: %v\n", err)
		os.Exit(1)
	}
	_, labelMap := labelOptions(labels)

	issue, err := createLinearTicket(apiKey, LinearTicket{
		Title:      title,
		TeamId:     teamId,
		Labels:     selections.Labels,
		Estimate:   selections.Estimate,
		AssigneeId: selections.AssigneeId,
		StatusId:   selections.StatusId,
	}, labelMap)
	if err != nil {
		fmt.Printf("❌ Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	branchName := fallbackBranchName(issue)
	issue.BranchName = branchName
	if jsonOutput {
		jsonData, err := json.Marshal(issue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(string(jsonData))
		return
	}

	if err := clipboard.WriteAll(branchName); err != nil {
		fmt.Println(branchName)
		fmt.Fprintf(os.Stderr, "❌ Failed to copy to clipboard: %v\n", err)
		return
	}

	fmt.Println(branchName)
}

func runConfigure(apiKey string) {
	fmt.Println("Configure default team, labels, estimate, and status")
	runSetTeam(apiKey)
	runSetLabels(apiKey)
	runSetEstimate()
	runSetStatus(apiKey)
}

func fallbackIssueBranchName(issue Issue) string {
	if issue.BranchName != "" {
		return issue.BranchName
	}

	return strings.ToLower(issue.Identifier)
}

func issueSearchScore(issue Issue, term string) int {
	query := strings.ToLower(strings.TrimSpace(term))
	if query == "" {
		return 0
	}

	identifier := strings.ToLower(issue.Identifier)
	title := strings.ToLower(issue.Title)
	searchText := identifier + " " + title
	if query == identifier {
		return 1000
	}
	if strings.Contains(identifier, query) {
		return 900 + len(query)
	}
	if strings.Contains(title, query) {
		return 700 + len(query)
	}
	if strings.Contains(searchText, query) {
		return 600 + len(query)
	}

	score := 0
	queryIndex := 0
	for _, r := range searchText {
		if queryIndex >= len(query) {
			break
		}
		if byte(r) == query[queryIndex] {
			score++
			queryIndex++
		}
	}
	if queryIndex != len(query) {
		return 0
	}

	return score
}

func findBestIssue(issues []Issue, term string) (Issue, bool) {
	var bestIssue Issue
	bestScore := 0
	for _, issue := range issues {
		score := issueSearchScore(issue, term)
		if score > bestScore {
			bestScore = score
			bestIssue = issue
		}
	}

	return bestIssue, bestScore > 0
}

func outputIssue(issue Issue, jsonOutput bool) {
	branchName := fallbackIssueBranchName(issue)
	issue.BranchName = branchName
	if jsonOutput {
		jsonData, err := json.Marshal(issue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(string(jsonData))
		return
	}

	if err := clipboard.WriteAll(branchName); err != nil {
		fmt.Println(branchName)
		fmt.Fprintf(os.Stderr, "❌ Failed to copy to clipboard: %v\n", err)
		return
	}

	fmt.Println(branchName)
}

func runIssueSearch(apiKey, searchTerm string, jsonOutput bool) {
	selections := loadUserSelections()
	teamId := requireDefaultTeam(selections)

	issues, err := fetchTeamIssues(apiKey, teamId)
	if err != nil {
		fmt.Printf("❌ Error fetching issues: %v\n", err)
		os.Exit(1)
	}
	if len(issues) == 0 {
		fmt.Println("No issues found for the default team")
		return
	}
	if searchTerm != "" {
		issue, found := findBestIssue(issues, searchTerm)
		if !found {
			fmt.Fprintf(os.Stderr, "No issue matched %q\n", searchTerm)
			os.Exit(1)
		}

		outputIssue(issue, jsonOutput)
		return
	}

	issueByKey := make(map[string]Issue, len(issues))
	options := make([]huh.Option[string], len(issues))
	for i, issue := range issues {
		key := issue.Identifier + " " + issue.Title
		issueByKey[key] = issue
		options[i] = huh.Option[string]{Key: key, Value: key}
	}

	selectedIssueKey := ""
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Issue").
				Description("Filter issues from the default team").
				Options(options...).
				Filtering(true).
				Value(&selectedIssueKey),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Println("Issue selection cancelled or error:", err)
		os.Exit(1)
	}

	issue := issueByKey[selectedIssueKey]
	outputIssue(issue, jsonOutput)
}

func runAuth(args []string) {
	if len(args) == 0 || hasHelpArg(args) {
		printAuthUsage()
		return
	}

	switch args[0] {
	case "login":
		if err := clearOAuthTokenCache(); err != nil {
			fmt.Printf("❌ Error clearing saved OAuth token: %v\n", err)
			os.Exit(1)
		}
		if _, err := runDCRLogin(oauthScopes()); err != nil {
			fmt.Printf("❌ Error signing in to Linear: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Linear OAuth token saved")
	case "logout":
		if err := clearOAuthTokenCache(); err != nil {
			fmt.Printf("❌ Error clearing saved OAuth token: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Linear OAuth token cleared")
	default:
		fmt.Printf("Unknown auth command: %s\n\n", args[0])
		printAuthUsage()
		os.Exit(1)
	}
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}

	return false
}

func printQuickUsage() {
	fmt.Println("Usage:")
	fmt.Println("  lnr quick [--json] <title>")
	fmt.Println("  lnr [--json] --quick <title>")
}

func printIssueUsage() {
	fmt.Println("Usage:")
	fmt.Println("  lnr issue [--json] [search term]")
	fmt.Println("  lnr [--json] issue [search term]")
}

func printCompletionUsage() {
	fmt.Println("Usage:")
	fmt.Println("  lnr completion bash")
	fmt.Println("  lnr completion zsh")
}

func printAuthUsage() {
	fmt.Println("Usage:")
	fmt.Println("  lnr auth login")
	fmt.Println("  lnr auth logout")
}

func parseQuickArgs(args []string) (string, bool) {
	var titleParts []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			titleParts = append(titleParts, arg)
		}
	}

	return strings.Join(titleParts, " "), jsonOutput
}

func parseIssueArgs(args []string) (string, bool) {
	var searchParts []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			searchParts = append(searchParts, arg)
		}
	}

	return strings.Join(searchParts, " "), jsonOutput
}

func printBashCompletion() {
	fmt.Print(`_lnr_completion() {
  local cur prev commands global_flags shells
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  commands="quick issue auth configure set-team set-labels set-estimate set-status completion reset help"
  global_flags="--clear-cache --json --quick -h --help"
  shells="bash zsh"

  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${commands} ${global_flags}" -- "${cur}") )
    return 0
  fi

  case "${COMP_WORDS[1]}" in
    quick)
      COMPREPLY=( $(compgen -W "--json -h --help" -- "${cur}") )
      return 0
      ;;
    issue)
      COMPREPLY=( $(compgen -W "--json -h --help" -- "${cur}") )
      return 0
      ;;
    auth)
      COMPREPLY=( $(compgen -W "login logout -h --help" -- "${cur}") )
      return 0
      ;;
    completion)
      COMPREPLY=( $(compgen -W "${shells}" -- "${cur}") )
      return 0
      ;;
  esac
}

complete -F _lnr_completion lnr
`)
}

func printZshCompletion() {
	fmt.Print(`#compdef lnr

_lnr() {
  local -a commands
  commands=(
    'quick:Create a Linear issue from a title'
    'issue:Find an issue in the default team'
    'auth:Manage OAuth sign-in'
    'configure:Configure default team, labels, estimate, and status'
    'set-team:Set the default team'
    'set-labels:Set default labels'
    'set-estimate:Set the default estimate'
    'set-status:Set the default status'
    'completion:Generate shell completions'
    'reset:Clear cached API data and saved defaults'
    'help:Show help'
  )

  case $words[2] in
    quick)
      _arguments '--json[Output JSON]' '-h[Show help]' '--help[Show help]' '*:title:'
      ;;
    issue)
      _arguments '--json[Output JSON]' '-h[Show help]' '--help[Show help]' '*:search term:'
      ;;
    auth)
      _arguments '1:auth command:(login logout)' '-h[Show help]' '--help[Show help]'
      ;;
    completion)
      _arguments '1:shell:(bash zsh)'
      ;;
    *)
      _arguments '--clear-cache[Clear cached API data and saved defaults]' '--json[Output JSON]' '--quick[Create a Linear issue from a title]' '1:command:->commands'
      if [[ $state == commands ]]; then
        _describe 'commands' commands
      fi
      ;;
  esac
}

_lnr "$@"
`)
}

func runCompletion(shell string) {
	switch shell {
	case "bash":
		printBashCompletion()
	case "zsh":
		printZshCompletion()
	default:
		printCompletionUsage()
		os.Exit(1)
	}
}

func main() {
	// Parse command-line flags
	clearCacheFlag := flag.Bool("clear-cache", false, "Clear cached API data and saved defaults")
	quickTitleFlag := flag.String("quick", "", "Create a Linear issue from a title and print the branch name")
	jsonOutputFlag := flag.Bool("json", false, "Output supported command result as JSON")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr quick [--json] <title>\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr issue [--json] [search term]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr auth login|logout\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr configure\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr set-team\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr set-labels\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr set-estimate\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr set-status\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr completion bash|zsh\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  lnr reset\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Handle clear cache flag
	if *clearCacheFlag {
		if err := resetData(); err != nil {
			fmt.Printf("❌ Error clearing data: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Data cleared successfully")
		return
	}
	if *quickTitleFlag != "" {
		runQuickCreate(getLinearAuthHeader(), *quickTitleFlag, *jsonOutputFlag)
		return
	}

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "quick":
			if len(args) == 1 || hasHelpArg(args[1:]) {
				printQuickUsage()
				return
			}
			title, jsonOutput := parseQuickArgs(args[1:])
			runQuickCreate(getLinearAuthHeader(), title, jsonOutput || *jsonOutputFlag)
		case "issue":
			if hasHelpArg(args[1:]) {
				printIssueUsage()
				return
			}
			searchTerm, jsonOutput := parseIssueArgs(args[1:])
			runIssueSearch(getLinearAuthHeader(), searchTerm, jsonOutput || *jsonOutputFlag)
		case "auth":
			runAuth(args[1:])
		case "configure":
			runConfigure(getLinearAuthHeader())
		case "completion":
			if len(args) < 2 || hasHelpArg(args[1:]) {
				printCompletionUsage()
				return
			}
			runCompletion(args[1])
		case "set-team":
			runSetTeam(getLinearAuthHeader())
		case "set-labels":
			runSetLabels(getLinearAuthHeader())
		case "set-estimate":
			runSetEstimate()
		case "set-status":
			runSetStatus(getLinearAuthHeader())
		case "reset":
			if err := resetData(); err != nil {
				fmt.Printf("❌ Error clearing data: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Data cleared successfully")
		case "help", "-h", "--help":
			flag.Usage()
		default:
			fmt.Printf("Unknown command: %s\n\n", args[0])
			flag.Usage()
			os.Exit(1)
		}
		return
	}

	var ticket LinearTicket
	selections := loadUserSelections()

	// Get API credentials
	apiKey := getLinearAuthHeader()

	// Fetch teams
	teams, err := loadTeams(apiKey)
	if err != nil {
		fmt.Printf("❌ Error fetching teams: %v\n", err)
		os.Exit(1)
	}

	// Create team selection options
	teamOptions := teamOptions(teams)

	// Select team - pre-select from cache and skip if already cached
	var selectedTeamId string = selections.TeamId
	if selectedTeamId == "" {
		// No cached team, show selection
		teamForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Team").
					Description("Select the team for this ticket").
					Options(teamOptions...).
					Value(&selectedTeamId),
			),
		)
		if err := teamForm.Run(); err != nil {
			fmt.Println("Team selection cancelled or error:", err)
			os.Exit(1)
		}
	} else {
		// Team is cached, verify it still exists
		teamExists := false
		for _, team := range teams {
			if team.ID == selectedTeamId {
				teamExists = true
				break
			}
		}
		if !teamExists {
			// Cached team no longer exists, show selection
			selectedTeamId = ""
			teamForm := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Team").
						Description("Select the team for this ticket").
						Options(teamOptions...).
						Value(&selectedTeamId),
				),
			)
			if err := teamForm.Run(); err != nil {
				fmt.Println("Team selection cancelled or error:", err)
				os.Exit(1)
			}
		}
	}

	// Find selected team
	var selectedTeam *Team
	for _, team := range teams {
		if team.ID == selectedTeamId {
			selectedTeam = &team
			break
		}
	}
	if selectedTeam == nil {
		fmt.Println("❌ Selected team not found")
		os.Exit(1)
	}

	// Fetch team labels, users, and workflow states
	var labels []Label
	var users []User
	var workflowStates []WorkflowState

	labels, err = loadTeamLabels(apiKey, selectedTeamId)
	if err != nil {
		fmt.Printf("❌ Error fetching labels: %v\n", err)
		os.Exit(1)
	}

	users, err = loadTeamUsers(apiKey, selectedTeamId)
	if err != nil {
		fmt.Printf("❌ Error fetching users: %v\n", err)
		os.Exit(1)
	}

	workflowStates, err = loadWorkflowStates(apiKey, selectedTeamId)
	if err != nil {
		fmt.Printf("❌ Error fetching workflow states: %v\n", err)
		os.Exit(1)
	}

	// Create options
	estimateOptions := getEstimateOptions(1) // Default to story points

	labelOptions, labelMap := labelOptions(labels)

	userOptions := make([]huh.Option[string], len(users)+1) // +1 for "No assignee"
	userOptions[0] = huh.Option[string]{Key: "No assignee", Value: ""}
	for i, user := range users {
		userOptions[i+1] = huh.Option[string]{Key: user.Name, Value: user.ID}
	}

	statusOptions := make([]huh.Option[string], len(workflowStates))
	for i, state := range workflowStates {
		statusOptions[i] = huh.Option[string]{Key: state.Name, Value: state.ID}
	}

	// Set default values from cache
	ticket.TeamId = selectedTeamId
	ticket.Estimate = selections.Estimate
	ticket.Labels = selections.Labels
	ticket.AssigneeId = selections.AssigneeId
	ticket.StatusId = selections.StatusId

	// Create the form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Ticket Title").
				Description("A brief summary of the issue or feature").
				Value(&ticket.Title).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("title cannot be empty")
					}
					return nil
				}),

			huh.NewText().
				Title("Description").
				Description("Detailed description of the ticket").
				Value(&ticket.Description).
				Lines(5),

			huh.NewSelect[string]().
				Title("Status").
				Description("Select the status for this ticket").
				Options(statusOptions...).
				Value(&ticket.StatusId),

			huh.NewSelect[string]().
				Title("Estimate").
				Description("Story point estimate").
				Options(estimateOptions...).
				Value(&ticket.Estimate),

			huh.NewMultiSelect[string]().
				Title("Labels").
				Description("Select applicable labels (space to toggle, enter to confirm)").
				Options(labelOptions...).
				Value(&ticket.Labels).
				Limit(4),

			huh.NewSelect[string]().
				Title("Assignee").
				Description("Select who should work on this ticket").
				Options(userOptions...).
				Value(&ticket.AssigneeId),
		),
	)

	// Run the form
	err = form.Run()
	if err != nil {
		fmt.Println("Form cancelled or error:", err)
		os.Exit(1)
	}

	// Display the collected information
	fmt.Println("\n" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📝 Ticket Information")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Title:       %s\n", ticket.Title)
	fmt.Printf("Description: %s\n", ticket.Description)

	// Show estimate with proper name
	estimateText := "No estimate"
	if ticket.Estimate != "" && ticket.Estimate != "0" {
		for _, option := range estimateOptions {
			if option.Value == ticket.Estimate {
				estimateText = option.Key
				break
			}
		}
	}
	fmt.Printf("Estimate:    %s\n", estimateText)

	// Show status name
	statusName := "Unknown"
	if ticket.StatusId != "" {
		for _, state := range workflowStates {
			if state.ID == ticket.StatusId {
				statusName = state.Name
				break
			}
		}
	}
	fmt.Printf("Status:      %s\n", statusName)

	// Show assignee name
	assigneeName := "No Assignee"
	if ticket.AssigneeId != "" {
		for _, user := range users {
			if user.ID == ticket.AssigneeId {
				assigneeName = user.Name
				break
			}
		}
	}
	fmt.Printf("Assignee:    %s\n", assigneeName)

	// Show labels
	if len(ticket.Labels) > 0 {
		fmt.Printf("Labels:      %s\n", strings.Join(ticket.Labels, ", "))
	} else {
		fmt.Printf("Labels:      None\n")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n🚀 Creating ticket in Linear...")
	issue, err := createLinearTicket(apiKey, ticket, labelMap)
	if err != nil {
		fmt.Printf("❌ Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Ticket created successfully! ID: %s\n", issue.Identifier)

	// Save user selections to cache
	selections = UserSelections{
		TeamId:     ticket.TeamId,
		AssigneeId: ticket.AssigneeId,
		Labels:     ticket.Labels,
		Estimate:   ticket.Estimate,
		StatusId:   ticket.StatusId,
	}
	saveUserSelections(selections)

	// Post-creation menu
	var action string
	postForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.Option[string]{Key: "Copy branch name", Value: "branch"},
					huh.Option[string]{Key: "Open in Linear", Value: "open"},
					huh.Option[string]{Key: "Exit", Value: "exit"},
				).
				Value(&action),
		),
	)

	if err := postForm.Run(); err != nil {
		fmt.Println("Menu cancelled or error:", err)
		return
	}

	switch action {
	case "branch":
		branchName := fallbackBranchName(issue)
		if err := clipboard.WriteAll(branchName); err != nil {
			fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
		} else {
			fmt.Printf("📋 Copied '%s' to clipboard\n", branchName)
		}
	case "open":
		// Get the full URL from the issue data
		url := fmt.Sprintf("https://linear.app/issue/%s", issue.Identifier)
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		case "darwin":
			cmd = exec.Command("open", url)
		case "linux":
			cmd = exec.Command("xdg-open", url)
		}
		if cmd != nil {
			if err := cmd.Run(); err != nil {
				fmt.Printf("❌ Failed to open URL: %v\n", err)
			}
		}
	case "exit":
		// Do nothing, just exit
	}
}

func createLinearTicket(apiKey string, ticket LinearTicket, labelMap map[string]string) (CreatedIssue, error) {
	if authHeader, ok := splitMCPAuthHeader(apiKey); ok {
		return createLinearTicketWithMCP(authHeader, ticket)
	}

	// GraphQL mutation to create an issue
	mutation := `
		mutation IssueCreate($input: IssueCreateInput!) {
			issueCreate(input: $input) {
				success
				issue {
					id
					identifier
					branchName
					title
					url
				}
			}
		}
	`

	// Prepare the input
	input := map[string]interface{}{
		"teamId":      ticket.TeamId,
		"title":       ticket.Title,
		"description": ticket.Description,
	}

	// Add estimate if provided
	if ticket.Estimate != "" && ticket.Estimate != "0" {
		if estimate, err := strconv.Atoi(ticket.Estimate); err == nil {
			input["estimate"] = estimate
		}
	}

	// Add labels if provided
	if len(ticket.Labels) > 0 {
		var labelIds []string
		for _, labelName := range ticket.Labels {
			if labelId, exists := labelMap[labelName]; exists {
				labelIds = append(labelIds, labelId)
			}
		}
		if len(labelIds) > 0 {
			input["labelIds"] = labelIds
		}
	}

	// Add assignee if provided
	if ticket.AssigneeId != "" {
		input["assigneeId"] = ticket.AssigneeId
	}

	// Add status if provided
	if ticket.StatusId != "" {
		input["stateId"] = ticket.StatusId
	}

	payload := map[string]interface{}{
		"query": mutation,
		"variables": map[string]interface{}{
			"input": input,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return CreatedIssue{}, err
	}

	// Make the API request
	req, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return CreatedIssue{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return CreatedIssue{}, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CreatedIssue{}, err
	}

	// Check for errors
	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		return CreatedIssue{}, fmt.Errorf("Linear API error: %v", errors)
	}

	// Extract issue ID
	data := result["data"].(map[string]interface{})
	issueCreate := data["issueCreate"].(map[string]interface{})
	issue := issueCreate["issue"].(map[string]interface{})

	return CreatedIssue{
		Identifier: issue["identifier"].(string),
		BranchName: getString(issue, "branchName"),
		Title:      issue["title"].(string),
		URL:        issue["url"].(string),
	}, nil
}
