package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestParseQuickArgs(t *testing.T) {
	title, jsonOutput := parseQuickArgs([]string{"--json", "Fix", "the", "thing"})
	if title != "Fix the thing" {
		t.Fatalf("expected title %q, got %q", "Fix the thing", title)
	}
	if !jsonOutput {
		t.Fatal("expected json output to be enabled")
	}
}

func TestParseQuickArgsTreatsOnlyJSONAsFlag(t *testing.T) {
	title, jsonOutput := parseQuickArgs([]string{"Fix", "--not-a-flag"})
	if title != "Fix --not-a-flag" {
		t.Fatalf("expected title %q, got %q", "Fix --not-a-flag", title)
	}
	if jsonOutput {
		t.Fatal("expected json output to be disabled")
	}
}

func TestParseIssueArgs(t *testing.T) {
	searchTerm, jsonOutput := parseIssueArgs([]string{"--json", "deployment", "check"})
	if searchTerm != "deployment check" {
		t.Fatalf("expected search term %q, got %q", "deployment check", searchTerm)
	}
	if !jsonOutput {
		t.Fatal("expected json output to be enabled")
	}
}

func TestHasHelpArg(t *testing.T) {
	if !hasHelpArg([]string{"--json", "--help"}) {
		t.Fatal("expected help arg to be detected")
	}
	if hasHelpArg([]string{"--json", "Fix", "thing"}) {
		t.Fatal("did not expect help arg to be detected")
	}
}

func TestFallbackBranchName(t *testing.T) {
	issue := CreatedIssue{Identifier: "PLT-123", BranchName: "plt-123-fix-the-thing"}
	if branchName := fallbackBranchName(issue); branchName != "plt-123-fix-the-thing" {
		t.Fatalf("expected branch name %q, got %q", "plt-123-fix-the-thing", branchName)
	}

	issue = CreatedIssue{Identifier: "PLT-123"}
	if branchName := fallbackBranchName(issue); branchName != "plt-123" {
		t.Fatalf("expected branch name %q, got %q", "plt-123", branchName)
	}
}

func TestFindBestIssue(t *testing.T) {
	issues := []Issue{
		{Identifier: "PLT-123", Title: "Fix deployment check"},
		{Identifier: "PLT-456", Title: "Update readme"},
	}

	issue, found := findBestIssue(issues, "deploy")
	if !found {
		t.Fatal("expected issue match")
	}
	if issue.Identifier != "PLT-123" {
		t.Fatalf("expected issue %q, got %q", "PLT-123", issue.Identifier)
	}
}

func TestFindBestIssueNoMatch(t *testing.T) {
	issues := []Issue{{Identifier: "PLT-123", Title: "Fix deployment check"}}
	_, found := findBestIssue(issues, "zzz")
	if found {
		t.Fatal("did not expect issue match")
	}
}

func TestFallbackIssueBranchName(t *testing.T) {
	issue := Issue{Identifier: "PLT-123", BranchName: "plt-123-fix-the-thing"}
	if branchName := fallbackIssueBranchName(issue); branchName != "plt-123-fix-the-thing" {
		t.Fatalf("expected branch name %q, got %q", "plt-123-fix-the-thing", branchName)
	}

	issue = Issue{Identifier: "PLT-123"}
	if branchName := fallbackIssueBranchName(issue); branchName != "plt-123" {
		t.Fatalf("expected branch name %q, got %q", "plt-123", branchName)
	}
}

func TestBearerAuthHeader(t *testing.T) {
	if got := bearerAuthHeader("token"); got != "Bearer token" {
		t.Fatalf("expected bearer token, got %q", got)
	}

	if got := bearerAuthHeader("Bearer token"); got != "Bearer token" {
		t.Fatalf("expected existing bearer header to be preserved, got %q", got)
	}
}

func TestMCPAuthHeader(t *testing.T) {
	header := mcpAuthHeader("token")
	authHeader, ok := splitMCPAuthHeader(header)
	if !ok {
		t.Fatal("expected MCP auth header")
	}
	if authHeader != "Bearer token" {
		t.Fatalf("expected bearer token, got %q", authHeader)
	}

	if authHeader, ok := splitMCPAuthHeader("lin_api_token"); ok || authHeader != "lin_api_token" {
		t.Fatalf("expected non-MCP auth header to be preserved, got %q, %v", authHeader, ok)
	}
}

func TestExtractSSEData(t *testing.T) {
	data, err := extractSSEData([]byte("event: message\ndata: {\"ok\":true}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("expected SSE data, got %q", string(data))
	}

	data, err = extractSSEData([]byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("expected raw JSON data, got %q", string(data))
	}
}

func TestCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := codeChallenge(verifier); got != expected {
		t.Fatalf("expected code challenge %q, got %q", expected, got)
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	oldAuthorizeURL := linearOAuthAuthorizeURL
	oldResource := linearOAuthResource
	t.Cleanup(func() {
		linearOAuthAuthorizeURL = oldAuthorizeURL
		linearOAuthResource = oldResource
	})

	linearOAuthAuthorizeURL = "https://example.com/authorize"
	linearOAuthResource = "https://example.com/resource"

	rawURL, err := buildAuthorizationURL("client-id", "http://127.0.0.1:1234/oauth/callback", "read write", "state", "verifier")
	if err != nil {
		t.Fatal(err)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}

	query := parsedURL.Query()
	expectations := map[string]string{
		"client_id":             "client-id",
		"redirect_uri":          "http://127.0.0.1:1234/oauth/callback",
		"response_type":         "code",
		"scope":                 "read write",
		"state":                 "state",
		"code_challenge":        codeChallenge("verifier"),
		"code_challenge_method": "S256",
		"resource":              "https://example.com/resource",
	}

	for key, expected := range expectations {
		if got := query.Get(key); got != expected {
			t.Fatalf("expected %s %q, got %q", key, expected, got)
		}
	}
}

func TestOAuthCallbackHandlerAcceptsCode(t *testing.T) {
	resultCh := make(chan oauthCallbackResult, 1)
	handler := oauthCallbackHandler("expected-state", resultCh)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=abc123&state=expected-state", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("expected no callback error, got %v", result.err)
	}
	if result.code != "abc123" {
		t.Fatalf("expected code %q, got %q", "abc123", result.code)
	}
}

func TestOAuthTokenCachePermissions(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	err := saveOAuthTokenCache(OAuthTokenCache{
		AccessToken: "access-token",
		Scope:       "read write",
		ClientID:    "client-id",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	cache, found := loadOAuthTokenCache("read write")
	if !found {
		t.Fatal("expected cached token to load")
	}
	if cache.AccessToken != "access-token" {
		t.Fatalf("expected cached access token, got %q", cache.AccessToken)
	}

	info, err := os.Stat(getCachePath(oauthTokenCacheKey))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected token cache permissions 0600, got %o", got)
	}
}
