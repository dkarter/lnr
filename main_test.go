package main

import (
	"bytes"
	"io"
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

func stubCommandHandlers(executions *int) commandHandlers {
	run := func() { (*executions)++ }
	runWithAuth := func(string) { run() }
	return commandHandlers{
		authHeader: func() string {
			run()
			return "test-auth"
		},
		quick:       func(string, string, bool) { run() },
		create:      func(string, IssueCreateOptions) { run() },
		issue:       func(string, string, bool) { run() },
		form:        run,
		login:       run,
		logout:      run,
		configure:   runWithAuth,
		setTeam:     runWithAuth,
		setLabels:   runWithAuth,
		setEstimate: run,
		setStatus:   runWithAuth,
		reset: func() error {
			run()
			return nil
		},
		skill: func(w io.Writer) {
			run()
			printSkill(w)
		},
	}
}

func executeCommand(t *testing.T, handlers commandHandlers, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	cmd := newRootCommand(handlers)
	cmd.SetArgs(args)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	err := cmd.Execute()
	return output.String(), err
}

func TestRootHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"help"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			executions := 0
			output, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{
				"lnr is a focused Linear CLI",
				"Available Commands:",
				"quick",
				"issue",
				"is",
				"ic",
				"auth",
				"--clear-cache",
				"lnr quick --json",
			} {
				if !strings.Contains(output, expected) {
					t.Errorf("expected root help to contain %q, got:\n%s", expected, output)
				}
			}
			if executions != 0 {
				t.Fatalf("expected root help not to execute handlers, got %d executions", executions)
			}
		})
	}
}

func TestEveryCommandHelpDoesNotExecute(t *testing.T) {
	paths := [][]string{
		{"quick"}, {"issue"}, {"issue", "create"}, {"issue", "search"}, {"ic"}, {"is"},
		{"auth"}, {"auth", "login"}, {"auth", "logout"},
		{"configure"}, {"set-team"}, {"set-labels"}, {"set-estimate"}, {"set-status"},
		{"completion"}, {"reset"}, {"skill"},
	}
	for _, path := range paths {
		t.Run(strings.Join(path, "_"), func(t *testing.T) {
			executions := 0
			args := append(append([]string(nil), path...), "--help")
			output, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, "Usage:") {
				t.Fatalf("expected command help, got:\n%s", output)
			}
			if executions != 0 {
				t.Fatalf("expected help not to execute handlers, got %d executions", executions)
			}
		})
	}
}

func TestIssueCommandHelpDescribesBranchNameOutput(t *testing.T) {
	paths := [][]string{
		{"quick"},
		{"issue", "create"},
		{"ic"},
		{"issue", "search"},
		{"is"},
	}
	for _, path := range paths {
		t.Run(strings.Join(path, "_"), func(t *testing.T) {
			executions := 0
			args := append(append([]string(nil), path...), "--help")
			output, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, "Linear branch name by default") {
				t.Fatalf("expected branch-name behavior in help, got:\n%s", output)
			}
			if executions != 0 {
				t.Fatalf("expected help not to execute handlers, got %d executions", executions)
			}
		})
	}
}

func TestUnknownCommand(t *testing.T) {
	output, err := runCLI(t, "not-a-command")
	if err == nil {
		t.Fatal("expected unknown command to fail")
	}
	if !strings.Contains(output, `unknown command "not-a-command" for "lnr"`) {
		t.Fatalf("expected unknown command error, got:\n%s", output)
	}
}

func TestCompletionShells(t *testing.T) {
	tests := map[string]string{
		"bash":       "__start_lnr",
		"zsh":        "#compdef lnr",
		"fish":       "complete -c lnr",
		"powershell": "Register-ArgumentCompleter -CommandName 'lnr'",
	}
	for shell, marker := range tests {
		t.Run(shell, func(t *testing.T) {
			executions := 0
			output, err := executeCommand(t, stubCommandHandlers(&executions), "completion", shell)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, marker) {
				t.Errorf("expected %s completion to contain %q", shell, marker)
			}
			if executions != 0 {
				t.Fatalf("completion unexpectedly executed a business handler")
			}
		})
	}
}

func TestCommandArgumentValidationDoesNotExecute(t *testing.T) {
	tests := [][]string{
		{"quick"},
		{"issue", "create", "extra"},
		{"issue", "create", "--description", "Missing title"},
		{"ic", "extra"},
		{"issue", "deployment"},
		{"auth", "login", "extra"},
		{"completion"},
		{"completion", "unsupported"},
		{"reset", "extra"},
		{"skill", "extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			executions := 0
			_, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err == nil {
				t.Fatal("expected argument validation to fail")
			}
			if executions != 0 {
				t.Fatalf("validation failure executed %d handlers", executions)
			}
		})
	}
}

func TestSkillCommandOutput(t *testing.T) {
	executions := 0
	output, err := executeCommand(t, stubCommandHandlers(&executions), "skill")
	if err != nil {
		t.Fatal(err)
	}
	if output != lnrSkill {
		t.Fatal("expected skill command to print the embedded skill")
	}
	if executions != 1 {
		t.Fatalf("expected only the skill handler to execute, got %d executions", executions)
	}
}

func TestLegacyRootFlags(t *testing.T) {
	t.Run("quick", func(t *testing.T) {
		var auth, title string
		var jsonOutput bool
		handlers := stubCommandHandlers(new(int))
		handlers.authHeader = func() string { return "legacy-auth" }
		handlers.quick = func(gotAuth, gotTitle string, gotJSON bool) {
			auth, title, jsonOutput = gotAuth, gotTitle, gotJSON
		}

		_, err := executeCommand(t, handlers, "--json", "--quick", "Fix the thing")
		if err != nil {
			t.Fatal(err)
		}
		if auth != "legacy-auth" || title != "Fix the thing" || !jsonOutput {
			t.Fatalf("unexpected legacy quick values: auth=%q title=%q json=%v", auth, title, jsonOutput)
		}
	})

	t.Run("json before subcommand", func(t *testing.T) {
		var title string
		var jsonOutput bool
		handlers := stubCommandHandlers(new(int))
		handlers.authHeader = func() string { return "legacy-auth" }
		handlers.quick = func(_ string, gotTitle string, gotJSON bool) {
			title, jsonOutput = gotTitle, gotJSON
		}

		_, err := executeCommand(t, handlers, "--json", "quick", "Fix", "the", "thing")
		if err != nil {
			t.Fatal(err)
		}
		if title != "Fix the thing" || !jsonOutput {
			t.Fatalf("unexpected legacy subcommand values: title=%q json=%v", title, jsonOutput)
		}
	})

	t.Run("clear cache", func(t *testing.T) {
		resets := 0
		handlers := stubCommandHandlers(new(int))
		handlers.reset = func() error {
			resets++
			return nil
		}
		_, err := executeCommand(t, handlers, "--clear-cache")
		if err != nil {
			t.Fatal(err)
		}
		if resets != 1 {
			t.Fatalf("expected one reset, got %d", resets)
		}
	})
}

func TestQuickAndIssueParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "quick", args: []string{"quick", "--json", "Fix", "the", "thing"}, want: "Fix the thing"},
		{name: "issue search", args: []string{"issue", "search", "--json", "deployment", "check"}, want: "deployment check"},
		{name: "is alias", args: []string{"is", "--json", "deployment", "check"}, want: "deployment check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var text string
			var jsonOutput bool
			handlers := stubCommandHandlers(new(int))
			handlers.authHeader = func() string { return "auth" }
			handlers.quick = func(_ string, value string, json bool) { text, jsonOutput = value, json }
			handlers.issue = func(_ string, value string, json bool) { text, jsonOutput = value, json }
			_, err := executeCommand(t, handlers, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if text != tt.want || !jsonOutput {
				t.Fatalf("got text=%q json=%v", text, jsonOutput)
			}
		})
	}
}

func TestBareIssueShowsHelpWithoutSearching(t *testing.T) {
	executions := 0
	output, err := executeCommand(t, stubCommandHandlers(&executions), "issue")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"create", "search"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected issue help to list %q, got:\n%s", expected, output)
		}
	}
	if executions != 0 {
		t.Fatalf("expected issue help not to execute handlers, got %d executions", executions)
	}
}

func TestIssueCreatePaths(t *testing.T) {
	for _, args := range [][]string{{"issue", "create"}, {"ic"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			executions := 0
			handlers := stubCommandHandlers(&executions)
			_, err := executeCommand(t, handlers, args...)
			if err != nil {
				t.Fatal(err)
			}
			if executions != 1 {
				t.Fatalf("expected only issue creation to execute, got %d handlers", executions)
			}
		})
	}
}

func TestIssueCreateNonInteractive(t *testing.T) {
	for _, args := range [][]string{
		{"issue", "create", "--title", "Fix deployment", "--description", "More details", "--json"},
		{"ic", "--title", "Fix deployment", "--description", "More details", "--json"},
	} {
		t.Run(args[0], func(t *testing.T) {
			var auth string
			var options IssueCreateOptions
			handlers := stubCommandHandlers(new(int))
			handlers.authHeader = func() string { return "test-auth" }
			handlers.create = func(gotAuth string, gotOptions IssueCreateOptions) {
				auth, options = gotAuth, gotOptions
			}

			_, err := executeCommand(t, handlers, args...)
			if err != nil {
				t.Fatal(err)
			}
			if auth != "test-auth" || options.Title != "Fix deployment" || options.Description != "More details" || !options.JSON {
				t.Fatalf("unexpected non-interactive create values: auth=%q options=%+v", auth, options)
			}
		})
	}
}

func TestRootJSONReachesNestedIssueCommands(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		var jsonOutput bool
		handlers := stubCommandHandlers(new(int))
		handlers.authHeader = func() string { return "auth" }
		handlers.issue = func(_ string, _ string, json bool) { jsonOutput = json }
		_, err := executeCommand(t, handlers, "--json", "issue", "search", "deployment")
		if err != nil {
			t.Fatal(err)
		}
		if !jsonOutput {
			t.Fatal("expected root --json to reach issue search")
		}
	})

	t.Run("create", func(t *testing.T) {
		var options IssueCreateOptions
		handlers := stubCommandHandlers(new(int))
		handlers.authHeader = func() string { return "auth" }
		handlers.create = func(_ string, got IssueCreateOptions) { options = got }
		_, err := executeCommand(t, handlers, "--json", "issue", "create", "--title", "Fix deployment")
		if err != nil {
			t.Fatal(err)
		}
		if !options.JSON {
			t.Fatal("expected root --json to reach issue create")
		}
	})
}

func TestQuickPreservesFlagLikeTitleWords(t *testing.T) {
	var title string
	handlers := stubCommandHandlers(new(int))
	handlers.authHeader = func() string { return "auth" }
	handlers.quick = func(_ string, value string, _ bool) { title = value }

	_, err := executeCommand(t, handlers, "quick", "Fix", "--not-a-flag")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Fix --not-a-flag" {
		t.Fatalf("expected flag-like title word to be preserved, got %q", title)
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
