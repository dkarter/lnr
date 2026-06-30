package main

import "testing"

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
