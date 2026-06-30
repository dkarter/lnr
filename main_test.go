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
