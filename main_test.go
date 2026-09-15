package main

import (
	"bytes"
	"encoding/json"
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

	tea "charm.land/bubbletea/v2"
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
		update:        func(string, IssueUpdateOptions) error { run(); return nil },
		delete:        func(string, IssueDeleteOptions) error { run(); return nil },
		confirmDelete: func(string) (bool, error) { run(); return true, nil },
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
		{"quick"}, {"issue"}, {"issue", "create"}, {"issue", "search"}, {"issue", "update"}, {"issue", "delete"}, {"ic"}, {"is"},
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
		{"issue", "update"},
		{"issue", "update", "PLT-123"},
		{"issue", "delete"},
		{"issue", "delete", "PLT-123", "--json"},
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

func TestIssueCreateAcceptsProject(t *testing.T) {
	var options IssueCreateOptions
	handlers := stubCommandHandlers(new(int))
	handlers.create = func(_ string, got IssueCreateOptions) { options = got }
	if _, err := executeCommand(t, handlers, "issue", "create", "--title", "Fix deployment", "--project", "Release"); err != nil {
		t.Fatal(err)
	}
	if options.Project != "Release" {
		t.Fatalf("expected project selection, got %+v", options)
	}
}

func TestProjectOptionsIncludeNoProjectSelection(t *testing.T) {
	options := projectOptions([]Project{{ID: "project-1", Name: "Release"}}, true)
	if len(options) != 2 || options[0].Key != "No project" || options[0].Value != "" || options[1].Value != "project-1" {
		t.Fatalf("unexpected project picker options: %+v", options)
	}
}

func TestFindProjectRequiresIDForAmbiguousName(t *testing.T) {
	projects := []Project{{ID: "project-1", Name: "Release"}, {ID: "project-2", Name: "Release"}}
	if _, err := findProject(projects, "Release"); err == nil || !strings.Contains(err.Error(), "use a project ID") {
		t.Fatalf("expected ambiguous project error, got %v", err)
	}
	if id, err := findProject(projects, "project-2"); err != nil || id != "project-2" {
		t.Fatalf("expected exact project ID, got id=%q err=%v", id, err)
	}
}

func TestIssueUpdateFlags(t *testing.T) {
	var options IssueUpdateOptions
	handlers := stubCommandHandlers(new(int))
	handlers.update = func(_ string, got IssueUpdateOptions) error { options = got; return nil }
	_, err := executeCommand(t, handlers, "issue", "update", "PLT-123",
		"--title", "New title", "--description=", "--team", "Platform", "--status", "Done", "--project", "Release", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if options.Identifier != "PLT-123" || options.Title != "New title" || options.Description != "" || options.Team != "Platform" || options.Status != "Done" || options.Project != "Release" || !options.TitleChanged || !options.DescriptionChanged || !options.TeamChanged || !options.StatusChanged || !options.ProjectChanged || !options.JSON {
		t.Fatalf("unexpected update options: %+v", options)
	}
}

func TestIssueUpdateProjectValidation(t *testing.T) {
	for _, args := range [][]string{
		{"issue", "update", "PLT-123", "--project", "Release", "--no-project"},
		{"issue", "update", "PLT-123", "--title="},
	} {
		executions := 0
		_, err := executeCommand(t, stubCommandHandlers(&executions), args...)
		if err == nil {
			t.Fatalf("expected validation failure for %v", args)
		}
		if executions != 0 {
			t.Fatalf("validation failure executed %d handlers", executions)
		}
	}
}

func TestIssueUpdateClearProjectIsExplicit(t *testing.T) {
	var options IssueUpdateOptions
	handlers := stubCommandHandlers(new(int))
	handlers.update = func(_ string, got IssueUpdateOptions) error { options = got; return nil }
	if _, err := executeCommand(t, handlers, "issue", "update", "PLT-123", "--no-project"); err != nil {
		t.Fatal(err)
	}
	if !options.ProjectChanged || !options.ClearProject || options.Project != "" {
		t.Fatalf("expected explicit project clearing, got %+v", options)
	}
}

func TestIssueDeleteConfirmation(t *testing.T) {
	t.Run("declined", func(t *testing.T) {
		deletes := 0
		handlers := stubCommandHandlers(new(int))
		handlers.confirmDelete = func(string) (bool, error) { return false, nil }
		handlers.delete = func(string, IssueDeleteOptions) error { deletes++; return nil }
		output, err := executeCommand(t, handlers, "issue", "delete", "PLT-123")
		if err != nil {
			t.Fatal(err)
		}
		if deletes != 0 || output != "Deletion cancelled\n" {
			t.Fatalf("unexpected cancellation: deletes=%d output=%q", deletes, output)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		handlers := stubCommandHandlers(new(int))
		handlers.confirmDelete = func(string) (bool, error) { return false, fmt.Errorf("user aborted") }
		_, err := executeCommand(t, handlers, "issue", "delete", "PLT-123")
		if err == nil || !strings.Contains(err.Error(), "confirmation cancelled") {
			t.Fatalf("expected cancellation error, got %v", err)
		}
	})

	t.Run("force json never prompts", func(t *testing.T) {
		prompts := 0
		var options IssueDeleteOptions
		handlers := stubCommandHandlers(new(int))
		handlers.confirmDelete = func(string) (bool, error) { prompts++; return true, nil }
		handlers.delete = func(_ string, got IssueDeleteOptions) error { options = got; return nil }
		if _, err := executeCommand(t, handlers, "issue", "delete", "PLT-123", "--force", "--json"); err != nil {
			t.Fatal(err)
		}
		if prompts != 0 || !options.Force || !options.JSON {
			t.Fatalf("unexpected forced deletion: prompts=%d options=%+v", prompts, options)
		}
	})
}

func TestIssueMutationHandlerFailure(t *testing.T) {
	handlers := stubCommandHandlers(new(int))
	handlers.update = func(string, IssueUpdateOptions) error { return fmt.Errorf("Linear unavailable") }
	_, err := executeCommand(t, handlers, "issue", "update", "PLT-123", "--status", "Done")
	if err == nil || err.Error() != "Linear unavailable" {
		t.Fatalf("expected API failure, got %v", err)
	}
}

func mcpResponse(t *testing.T, writer http.ResponseWriter, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(writer).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"result": map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": string(data)}},
		},
	})
}

func TestUpdateIssueValidatesRelationshipsAndSendsMutation(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	oldResource := linearOAuthResource
	t.Cleanup(func() { linearOAuthResource = oldResource })
	var saved map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		switch body.Params.Name {
		case "get_issue":
			mcpResponse(t, writer, MCPIssue{ID: "PLT-123", UUID: "uuid-123", Title: "Old", Team: "Old", TeamID: "old-team"})
		case "list_teams":
			mcpResponse(t, writer, MCPPage[Team]{Teams: []Team{{ID: "team-1", Name: "Platform"}}})
		case "list_issue_statuses":
			mcpResponse(t, writer, []WorkflowState{{ID: "status-1", Name: "Done"}})
		case "list_projects":
			mcpResponse(t, writer, MCPPage[Project]{Projects: []Project{{ID: "project-1", Name: "Release"}}})
		case "save_issue":
			saved = body.Params.Arguments
			mcpResponse(t, writer, MCPIssue{ID: "PLT-123", Title: "New"})
		default:
			t.Errorf("unexpected MCP tool %q", body.Params.Name)
		}
	}))
	defer server.Close()
	linearOAuthResource = server.URL

	issue, err := updateIssue(mcpAuthHeader("token"), IssueUpdateOptions{
		Identifier: "PLT-123", Title: "New", Team: "Platform", Status: "Done", Project: "Release",
		TitleChanged: true, TeamChanged: true, StatusChanged: true, ProjectChanged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Identifier != "PLT-123" || saved["id"] != "PLT-123" || saved["team"] != "team-1" || saved["state"] != "status-1" || saved["project"] != "project-1" || saved["title"] != "New" {
		t.Fatalf("unexpected saved issue or mutation: issue=%+v mutation=%+v", issue, saved)
	}
}

func TestUpdateIssueRejectsProjectOutsideTeam(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	oldResource := linearOAuthResource
	t.Cleanup(func() { linearOAuthResource = oldResource })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body.Params.Name == "get_issue" {
			mcpResponse(t, writer, MCPIssue{ID: "PLT-123", TeamID: "team-1"})
			return
		}
		mcpResponse(t, writer, MCPPage[Project]{})
	}))
	defer server.Close()
	linearOAuthResource = server.URL

	_, err := updateIssue(mcpAuthHeader("token"), IssueUpdateOptions{Identifier: "PLT-123", Project: "Other", ProjectChanged: true})
	if err == nil || !strings.Contains(err.Error(), "not available to the target team") {
		t.Fatalf("expected actionable project error, got %v", err)
	}
}

func TestUpdateIssueRejectsStatusOutsideTeam(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	oldResource := linearOAuthResource
	t.Cleanup(func() { linearOAuthResource = oldResource })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body.Params.Name == "get_issue" {
			mcpResponse(t, writer, MCPIssue{ID: "PLT-123", TeamID: "team-1"})
			return
		}
		mcpResponse(t, writer, []WorkflowState{})
	}))
	defer server.Close()
	linearOAuthResource = server.URL

	_, err := updateIssue(mcpAuthHeader("token"), IssueUpdateOptions{Identifier: "PLT-123", Status: "Other", StatusChanged: true})
	if err == nil || !strings.Contains(err.Error(), "not available to the target team") {
		t.Fatalf("expected actionable status error, got %v", err)
	}
}

func TestDeleteIssueRequiresAPIKeyForMCPLogin(t *testing.T) {
	err := deleteIssue(mcpAuthHeader("token"), "PLT-123")
	if err == nil || !strings.Contains(err.Error(), "set LINEAR_API_KEY") {
		t.Fatalf("expected actionable authentication error, got %v", err)
	}
}

func TestDeleteIssueUsesResolvedIDAndPropagatesFailure(t *testing.T) {
	oldGraphQL := linearGraphQLEndpoint
	t.Cleanup(func() {
		linearGraphQLEndpoint = oldGraphQL
	})

	fail := false
	graphqlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "api-key" {
			t.Errorf("unexpected authorization header %q", got)
		}
		var body struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		if strings.Contains(body.Query, "query Issue") {
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"data": map[string]interface{}{"issue": map[string]interface{}{"id": "uuid-123", "identifier": "PLT-123"}},
			})
			return
		}
		if body.Variables["id"] != "uuid-123" {
			t.Errorf("expected resolved issue ID, got %+v", body.Variables)
		}
		if fail {
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errors": []map[string]string{{"message": "permission denied"}}})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{"data": map[string]interface{}{"issueDelete": map[string]interface{}{"success": true}}})
	}))
	defer graphqlServer.Close()
	linearGraphQLEndpoint = graphqlServer.URL

	if err := deleteIssue("api-key", "PLT-123"); err != nil {
		t.Fatalf("expected successful deletion, got %v", err)
	}
	fail = true
	err := deleteIssue("api-key", "PLT-123")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected Linear API failure, got %v", err)
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

func TestIssueSearchInitialPage(t *testing.T) {
	var queries, cursors []string
	model := newIssueSearchModel(func(query, cursor string) (IssuePage, error) {
		queries = append(queries, query)
		cursors = append(cursors, cursor)
		return IssuePage{
			Issues:      []Issue{{Identifier: "PLT-1", Title: "First issue"}},
			HasNextPage: true,
			EndCursor:   "page-2",
		}, nil
	})

	msg := model.fetchPage("", model.requestID)()
	updated, _ := model.Update(msg)
	model = updated.(issueSearchModel)

	if len(queries) != 1 || queries[0] != "" || len(cursors) != 1 || cursors[0] != "" {
		t.Fatalf("expected one unfiltered initial request, got queries=%q cursors=%q", queries, cursors)
	}
	if len(model.list.Items()) != 1 || !model.hasNextPage || model.cursor != "page-2" {
		t.Fatalf("unexpected initial page state: items=%d next=%v cursor=%q", len(model.list.Items()), model.hasNextPage, model.cursor)
	}
}

func TestIssueSearchQueryFetchesFilteredFirstPage(t *testing.T) {
	var query, cursor string
	model := newIssueSearchModel(func(gotQuery, gotCursor string) (IssuePage, error) {
		query, cursor = gotQuery, gotCursor
		return IssuePage{Issues: []Issue{{Identifier: "PLT-2", Title: "Deployment check"}}}, nil
	})
	for _, char := range "deployment" {
		updated, _ := model.Update(tea.KeyPressMsg{Code: char, Text: string(char)})
		model = updated.(issueSearchModel)
	}

	updated, cmd := model.Update(issueSearchDebounceMsg{requestID: model.requestID})
	model = updated.(issueSearchModel)
	if cmd == nil {
		t.Fatal("expected filtered page request")
	}
	updated, _ = model.Update(cmd())
	model = updated.(issueSearchModel)

	if query != "deployment" || cursor != "" {
		t.Fatalf("expected filtered first page, got query=%q cursor=%q", query, cursor)
	}
	if len(model.list.Items()) != 1 {
		t.Fatalf("expected one filtered result, got %d", len(model.list.Items()))
	}
}

func TestIssueSearchAppendsSubsequentPage(t *testing.T) {
	calls := 0
	model := newIssueSearchModel(func(query, cursor string) (IssuePage, error) {
		calls++
		if cursor == "page-2" {
			return IssuePage{Issues: []Issue{{Identifier: "PLT-2", Title: "Second"}}}, nil
		}
		return IssuePage{
			Issues:      []Issue{{Identifier: "PLT-1", Title: "First"}},
			HasNextPage: true,
			EndCursor:   "page-2",
		}, nil
	})

	updated, _ := model.Update(model.fetchPage("", model.requestID)())
	model = updated.(issueSearchModel)
	updated, _ = model.Update(model.fetchPage(model.cursor, model.requestID)())
	model = updated.(issueSearchModel)

	if calls != 2 {
		t.Fatalf("expected two page requests, got %d", calls)
	}
	if len(model.list.Items()) != 2 || model.hasNextPage {
		t.Fatalf("expected appended final page, got items=%d next=%v", len(model.list.Items()), model.hasNextPage)
	}
}

func TestIssueSearchLoadsMoreAtEnd(t *testing.T) {
	model := newIssueSearchModel(func(string, string) (IssuePage, error) { return IssuePage{}, nil })
	updated, _ := model.Update(issuePageMsg{
		requestID: model.requestID,
		page: IssuePage{
			Issues:      []Issue{{Identifier: "PLT-1", Title: "First"}},
			HasNextPage: true,
			EndCursor:   "page-2",
		},
	})
	model = updated.(issueSearchModel)

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = updated.(issueSearchModel)
	if cmd == nil || !model.loading {
		t.Fatal("expected down at the end to request the next page")
	}
}

func TestIssueSearchIgnoresStaleResponse(t *testing.T) {
	model := newIssueSearchModel(func(string, string) (IssuePage, error) { return IssuePage{}, nil })
	staleRequestID := model.requestID
	model.beginSearch("new query")

	updated, _ := model.Update(issuePageMsg{
		requestID: staleRequestID,
		page:      IssuePage{Issues: []Issue{{Identifier: "OLD-1", Title: "Stale"}}},
	})
	model = updated.(issueSearchModel)

	if len(model.list.Items()) != 0 || !model.loading {
		t.Fatalf("stale response changed current search state: items=%d loading=%v", len(model.list.Items()), model.loading)
	}
}

func TestIssueSearchEmptyAndErrorStates(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		model := newIssueSearchModel(func(string, string) (IssuePage, error) { return IssuePage{}, nil })
		updated, _ := model.Update(model.fetchPage("", model.requestID)())
		model = updated.(issueSearchModel)
		if !strings.Contains(model.View().Content, "No matching issues found") {
			t.Fatalf("expected empty state, got %q", model.View().Content)
		}
	})

	t.Run("error", func(t *testing.T) {
		model := newIssueSearchModel(func(string, string) (IssuePage, error) {
			return IssuePage{}, fmt.Errorf("request failed")
		})
		updated, _ := model.Update(model.fetchPage("", model.requestID)())
		model = updated.(issueSearchModel)
		if !strings.Contains(model.View().Content, "Error fetching issues: request failed") {
			t.Fatalf("expected error state, got %q", model.View().Content)
		}
	})
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
