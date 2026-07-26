package main

import (
	"bytes"
	"fmt"
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
		promptTitle: func() (string, error) {
			run()
			return "Prompted title", nil
		},
		checkoutReady: func() error { run(); return nil },
		quick:         func(string, string, BranchOutputOptions) { run() },
		create:        func(string, IssueCreateOptions) { run() },
		issue:         func(string, string, BranchOutputOptions) { run() },
		form:          run,
		login:         run,
		logout:        run,
		configure:     runWithAuth,
		setTeam:       runWithAuth,
		setLabels:     runWithAuth,
		setEstimate:   run,
		setStatus:     runWithAuth,
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
				"config",
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

func TestRootVersion(t *testing.T) {
	executions := 0
	output, err := executeCommand(t, stubCommandHandlers(&executions), "--version")
	if err != nil {
		t.Fatal(err)
	}
	if output != "lnr version "+version+"\n" {
		t.Fatalf("expected version output, got %q", output)
	}
	if executions != 0 {
		t.Fatalf("expected version flag not to execute handlers, got %d executions", executions)
	}
}

func TestEveryCommandHelpDoesNotExecute(t *testing.T) {
	paths := [][]string{
		{"quick"}, {"issue"}, {"issue", "create"}, {"issue", "search"}, {"ic"}, {"is"},
		{"auth"}, {"auth", "login"}, {"auth", "logout"},
		{"config"}, {"config", "set-team"}, {"config", "set-labels"},
		{"config", "set-estimate"}, {"config", "set-status"},
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
		{"issue", "create", "extra"},
		{"issue", "create", "--description", "Missing title"},
		{"ic", "extra"},
		{"issue", "deployment"},
		{"config", "set-team", "extra"},
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
		handlers.quick = func(gotAuth, gotTitle string, output BranchOutputOptions) {
			auth, title, jsonOutput = gotAuth, gotTitle, output.JSON
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
		handlers.quick = func(_ string, gotTitle string, output BranchOutputOptions) {
			title, jsonOutput = gotTitle, output.JSON
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
			handlers.quick = func(_ string, value string, output BranchOutputOptions) { text, jsonOutput = value, output.JSON }
			handlers.issue = func(_ string, value string, output BranchOutputOptions) { text, jsonOutput = value, output.JSON }
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

func TestQuickPromptsForMissingTitle(t *testing.T) {
	var title string
	prompts := 0
	handlers := stubCommandHandlers(new(int))
	handlers.authHeader = func() string { return "auth" }
	handlers.promptTitle = func() (string, error) {
		prompts++
		return "Fix the prompted thing", nil
	}
	handlers.quick = func(_ string, value string, _ BranchOutputOptions) { title = value }

	_, err := executeCommand(t, handlers, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatalf("expected one title prompt, got %d", prompts)
	}
	if title != "Fix the prompted thing" {
		t.Fatalf("expected prompted title, got %q", title)
	}
}

func TestQuickPromptCancellationDoesNotPrintUsage(t *testing.T) {
	handlers := stubCommandHandlers(new(int))
	handlers.promptTitle = func() (string, error) { return "", fmt.Errorf("user aborted") }

	output, err := executeCommand(t, handlers, "quick")
	if err == nil || err.Error() != "user aborted" {
		t.Fatalf("expected cancellation error, got %v", err)
	}
	if output != "" {
		t.Fatalf("expected no usage output, got:\n%s", output)
	}
}

func TestQuickCheckout(t *testing.T) {
	var title string
	var checkout bool
	handlers := stubCommandHandlers(new(int))
	handlers.authHeader = func() string { return "auth" }
	handlers.quick = func(_ string, value string, output BranchOutputOptions) {
		title, checkout = value, output.Checkout
	}

	_, err := executeCommand(t, handlers, "quick", "-c", "Fix", "the", "thing")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Fix the thing" || !checkout {
		t.Fatalf("got title=%q checkout=%v", title, checkout)
	}
}

func TestQuickCheckoutHonorsFlagDelimiter(t *testing.T) {
	var title string
	var checkout bool
	handlers := stubCommandHandlers(new(int))
	handlers.authHeader = func() string { return "auth" }
	handlers.quick = func(_ string, value string, output BranchOutputOptions) {
		title, checkout = value, output.Checkout
	}

	_, err := executeCommand(t, handlers, "quick", "--", "Document", "--checkout", "behavior")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Document --checkout behavior" || checkout {
		t.Fatalf("got title=%q checkout=%v", title, checkout)
	}
}

func TestQuickCheckoutPreflightFailure(t *testing.T) {
	executions := 0
	handlers := stubCommandHandlers(&executions)
	handlers.checkoutReady = func() error { return fmt.Errorf("not a worktree") }

	_, err := executeCommand(t, handlers, "quick", "--checkout", "Fix it")
	if err == nil || !strings.Contains(err.Error(), "not a worktree") {
		t.Fatalf("expected preflight error, got %v", err)
	}
	if executions != 0 {
		t.Fatalf("checkout preflight failure executed %d handlers", executions)
	}
}

func TestQuickRejectsCheckoutWithJSON(t *testing.T) {
	executions := 0
	_, err := executeCommand(t, stubCommandHandlers(&executions), "quick", "--json", "--checkout", "Fix it")
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected conflicting flag error, got %v", err)
	}
	if executions != 0 {
		t.Fatalf("flag conflict executed %d handlers", executions)
	}
}

func TestBranchOutputFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "quick copy", args: []string{"quick", "--copy=true", "Fix it"}},
		{name: "create checkout", args: []string{"issue", "create", "-c", "--title", "Fix it"}},
		{name: "create alias checkout", args: []string{"ic", "--checkout", "--title", "Fix it"}},
		{name: "search checkout", args: []string{"issue", "search", "--checkout=true", "Fix it"}},
		{name: "search alias copy", args: []string{"is", "--copy", "Fix it"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output BranchOutputOptions
			handlers := stubCommandHandlers(new(int))
			handlers.authHeader = func() string { return "auth" }
			handlers.checkoutReady = func() error { return nil }
			handlers.quick = func(_ string, _ string, got BranchOutputOptions) { output = got }
			handlers.create = func(_ string, got IssueCreateOptions) { output = got.BranchOutputOptions }
			handlers.issue = func(_ string, _ string, got BranchOutputOptions) { output = got }

			if _, err := executeCommand(t, handlers, tt.args...); err != nil {
				t.Fatal(err)
			}
			wantCopy := strings.Contains(tt.name, "copy")
			wantCheckout := strings.Contains(tt.name, "checkout")
			if output.Copy != wantCopy || output.Checkout != wantCheckout {
				t.Fatalf("unexpected output options: %+v", output)
			}
		})
	}
}

func TestBranchOutputFlagsAreMutuallyExclusive(t *testing.T) {
	commands := [][]string{
		{"quick", "--copy", "--checkout", "Fix it"},
		{"issue", "create", "--json", "--copy", "--title", "Fix it"},
		{"issue", "search", "--json", "--checkout", "Fix it"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args[:2], "_"), func(t *testing.T) {
			executions := 0
			_, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
				t.Fatalf("expected conflicting flag error, got %v", err)
			}
			if executions != 0 {
				t.Fatalf("flag conflict executed %d handlers", executions)
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

func TestConfigCommandPaths(t *testing.T) {
	tests := [][]string{
		{"config"},
		{"config", "set-team"},
		{"config", "set-labels"},
		{"config", "set-estimate"},
		{"config", "set-status"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			executions := 0
			_, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err != nil {
				t.Fatal(err)
			}
			if executions == 0 {
				t.Fatal("expected config handler to execute")
			}
		})
	}
}

func TestLegacyConfigCommandsStillExecute(t *testing.T) {
	for _, args := range [][]string{{"configure"}, {"set-team"}, {"set-labels"}, {"set-estimate"}, {"set-status"}} {
		t.Run(args[0], func(t *testing.T) {
			executions := 0
			_, err := executeCommand(t, stubCommandHandlers(&executions), args...)
			if err != nil {
				t.Fatal(err)
			}
			if executions == 0 {
				t.Fatal("expected deprecated config command to execute")
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

func TestClearAccountDataRemovesCacheAndDefaults(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveToCache("teams", []Team{{ID: "old-team", Name: "Old"}}); err != nil {
		t.Fatal(err)
	}
	if err := saveUserSelections(UserSelections{TeamId: "old-team"}); err != nil {
		t.Fatal(err)
	}
	if err := saveOAuthTokenCache(OAuthTokenCache{AccessToken: "old-token"}); err != nil {
		t.Fatal(err)
	}
	cacheDir := getCacheDir()
	configDir := getConfigDir()

	if err := clearAccountData(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cacheDir, configDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got %v", path, err)
		}
	}
}

func TestRootJSONReachesNestedIssueCommands(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		var jsonOutput bool
		handlers := stubCommandHandlers(new(int))
		handlers.authHeader = func() string { return "auth" }
		handlers.issue = func(_ string, _ string, output BranchOutputOptions) { jsonOutput = output.JSON }
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
	handlers.quick = func(_ string, value string, _ BranchOutputOptions) { title = value }

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

func TestCheckoutBranch(t *testing.T) {
	repo := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v\n%s", err, output)
	}
	t.Chdir(repo)

	if err := checkGitWorktree(); err != nil {
		t.Fatal(err)
	}
	if err := checkoutBranch("plt-123-fix-the-thing"); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		t.Fatal(err)
	}
	if branch := strings.TrimSpace(string(output)); branch != "plt-123-fix-the-thing" {
		t.Fatalf("expected checked out branch, got %q", branch)
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
