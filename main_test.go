package main

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCLIHelper(t *testing.T) {
	if os.Getenv("LNR_TEST_HELPER") != "1" {
		return
	}

	separator := 0
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	os.Args = append([]string{"lnr"}, os.Args[separator+1:]...)
	flag.CommandLine = flag.NewFlagSet("lnr", flag.ExitOnError)
	main()
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	commandArgs := append([]string{"-test.run=^TestCLIHelper$", "--"}, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), "LNR_TEST_HELPER=1")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestExistingRootHelp(t *testing.T) {
	output, err := runCLI(t, "help")
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"lnr quick [--json] <title>",
		"lnr issue [--json] [search term]",
		"lnr auth login|logout",
		"lnr completion bash|zsh",
		"lnr skill",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected root help to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestExistingCommandHelp(t *testing.T) {
	tests := []struct {
		command string
		usage   string
	}{
		{command: "quick", usage: "lnr quick [--json] <title>"},
		{command: "issue", usage: "lnr issue [--json] [search term]"},
		{command: "auth", usage: "lnr auth login"},
		{command: "completion", usage: "lnr completion bash"},
		{command: "skill", usage: "lnr skill"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			output, err := runCLI(t, tt.command, "--help")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, tt.usage) {
				t.Fatalf("expected help to contain %q, got:\n%s", tt.usage, output)
			}
		})
	}
}

func TestUnknownCommand(t *testing.T) {
	output, err := runCLI(t, "not-a-command")
	if err == nil {
		t.Fatal("expected unknown command to fail")
	}
	if !strings.Contains(output, "Unknown command: not-a-command") {
		t.Fatalf("expected unknown command error, got:\n%s", output)
	}
	if !strings.Contains(output, "lnr quick") {
		t.Fatalf("expected root usage after unknown command, got:\n%s", output)
	}
}

func TestCompletionsDiscoverCommands(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			output, err := runCLI(t, "completion", shell)
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"quick", "issue", "auth", "skill"} {
				if !strings.Contains(output, command) {
					t.Errorf("expected %s completion to contain %q", shell, command)
				}
			}
		})
	}
}

func TestPrintSkill(t *testing.T) {
	var output bytes.Buffer
	printSkill(&output)

	if output.String() != lnrSkill {
		t.Fatal("expected skill command to print the embedded skill")
	}
	if !bytes.Contains(output.Bytes(), []byte("name: lnr")) {
		t.Fatal("expected embedded skill metadata")
	}
}

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
