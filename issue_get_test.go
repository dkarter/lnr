package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssueGetCommand(t *testing.T) {
	for _, args := range [][]string{{"issue", "get", "PLT-123", "--json"}, {"--json", "issue", "get", "PLT-123"}, {"issue", "get", "PLT-123"}} {
		executions := 0
		handlers := stubCommandHandlers(&executions)
		handlers.get = func(auth, identifier string) (IssueDetails, error) {
			if auth != "test-auth" || identifier != "PLT-123" {
				t.Fatalf("unexpected lookup: %q %q", auth, identifier)
			}
			return IssueDetails{Issue: Issue{Identifier: identifier, Title: "Fix deployment"}, Status: WorkflowState{Name: "Shipped", Type: "completed"}}, nil
		}
		output, err := executeCommand(t, handlers, args...)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(args, " "), "--json") {
			var issue IssueDetails
			if err := json.Unmarshal([]byte(output), &issue); err != nil || issue.Identifier != "PLT-123" || issue.Status.Type != "completed" {
				t.Fatalf("unexpected JSON: %s, %v", output, err)
			}
		} else if !strings.Contains(output, "Shipped (completed)") {
			t.Fatalf("unexpected text: %s", output)
		}
		if executions != 1 {
			t.Fatalf("unexpected handlers called: %d", executions)
		}
	}
}

func TestIssueGetRejectsInvalidArgsBeforeAuth(t *testing.T) {
	for _, args := range [][]string{{"issue", "get"}, {"issue", "get", " "}, {"issue", "get", "PLT-1", "PLT-2"}} {
		executions := 0
		if _, err := executeCommand(t, stubCommandHandlers(&executions), args...); err == nil || executions != 0 {
			t.Fatalf("expected validation before auth: %v (%d)", err, executions)
		}
	}
}

func TestIssueGetPropagatesErrors(t *testing.T) {
	executions := 0
	handlers := stubCommandHandlers(&executions)
	handlers.get = func(string, string) (IssueDetails, error) { return IssueDetails{}, fmt.Errorf("issue unavailable") }
	output, err := executeCommand(t, handlers, "issue", "get", "PLT-123", "--json")
	if err == nil || !strings.Contains(err.Error(), "issue unavailable") || strings.Contains(output, `"issueId"`) {
		t.Fatalf("unexpected result: %s, %v", output, err)
	}
}

func TestIssueGetAuthFailureDoesNotFetch(t *testing.T) {
	executions := 0
	handlers := stubCommandHandlers(&executions)
	handlers.getAuth = func() (string, error) { return "", fmt.Errorf("sign in separately") }
	handlers.get = func(string, string) (IssueDetails, error) {
		t.Fatal("must not fetch without auth")
		return IssueDetails{}, nil
	}
	_, err := executeCommand(t, handlers, "issue", "get", "PLT-123", "--json")
	if err == nil || !strings.Contains(err.Error(), "sign in separately") || executions != 0 {
		t.Fatalf("unexpected auth failure: %v (%d)", err, executions)
	}
}

func TestNonInteractiveAuthWithoutCredentials(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "")
	t.Setenv("LINEAR_OAUTH_ACCESS_TOKEN", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	header, err := getNonInteractiveAuthHeader()
	if err == nil || header != "" || !strings.Contains(err.Error(), "lnr auth login") {
		t.Fatal("expected noninteractive authentication failure")
	}
}

func TestGetIssueGraphQLFailures(t *testing.T) {
	oldEndpoint := linearGraphQLEndpoint
	t.Cleanup(func() { linearGraphQLEndpoint = oldEndpoint })
	for _, response := range []string{`{"data":{"issue":null}}`, `{"errors":[{"message":"not authorized"}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
		linearGraphQLEndpoint = server.URL
		if _, err := getIssue("test-key", "PLT-123"); err == nil {
			t.Fatalf("expected error for %s", response)
		}
		server.Close()
	}
}

func TestGetIssueMCPFailures(t *testing.T) {
	oldResource := linearOAuthResource
	t.Cleanup(func() { linearOAuthResource = oldResource })
	for _, failedTool := range []string{"get_issue", "list_issue_statuses", "missing issue", "missing status", "unknown status"} {
		t.Run(failedTool, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Params struct{ Name string } }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Params.Name == failedTool {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				if body.Params.Name == "list_issue_statuses" {
					mcpResponse(t, w, []WorkflowState{})
					return
				}
				issue := MCPIssue{ID: "PLT-123", Team: "Platform", Status: "Unknown"}
				if failedTool == "missing issue" {
					issue.ID = ""
				}
				if failedTool == "missing status" {
					issue.Status = ""
				}
				mcpResponse(t, w, issue)
			}))
			defer server.Close()
			linearOAuthResource = server.URL
			if _, err := getIssue(mcpAuthHeader("test-token"), "PLT-123"); err == nil {
				t.Fatal("expected lookup failure")
			}
		})
	}
}

func TestGetIssueGraphQL(t *testing.T) {
	oldEndpoint := linearGraphQLEndpoint
	t.Cleanup(func() { linearGraphQLEndpoint = oldEndpoint })
	for _, stateType := range []string{"started", "completed", "canceled", ""} {
		t.Run(stateType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Query     string
					Variables map[string]string
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Variables["id"] != "PLT-123" || !strings.Contains(body.Query, "state { id name type }") {
					t.Errorf("unexpected query: %+v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"issue": map[string]any{"identifier": "PLT-123", "title": "Fix", "state": WorkflowState{ID: "state-1", Name: "Custom status", Type: stateType}}}})
			}))
			defer server.Close()
			linearGraphQLEndpoint = server.URL
			issue, err := getIssue("test-key", "PLT-123")
			if stateType == "" {
				if err == nil {
					t.Fatal("expected missing status error")
				}
				return
			}
			if err != nil || issue.Identifier != "PLT-123" || issue.Status.Type != stateType {
				t.Fatalf("unexpected issue: %+v, %v", issue, err)
			}
		})
	}
}

func TestGetIssueMCP(t *testing.T) {
	oldResource := linearOAuthResource
	t.Cleanup(func() { linearOAuthResource = oldResource })
	for _, statusType := range []string{"started", "completed", "canceled", ""} {
		t.Run(statusType, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Params struct {
						Name      string
						Arguments map[string]string
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				calls++
				switch body.Params.Name {
				case "get_issue":
					if body.Params.Arguments["id"] != "PLT-123" {
						t.Error("incorrect issue")
					}
					mcpResponse(t, w, MCPIssue{ID: "PLT-123", Title: "Fix", TeamID: "team-1", Status: "Custom status"})
				case "list_issue_statuses":
					if body.Params.Arguments["team"] != "team-1" {
						t.Error("incorrect team")
					}
					mcpResponse(t, w, []WorkflowState{{ID: "state-1", Name: "Custom status", Type: statusType}})
				default:
					t.Errorf("unexpected tool: %s", body.Params.Name)
				}
			}))
			defer server.Close()
			linearOAuthResource = server.URL
			issue, err := getIssue(mcpAuthHeader("test-token"), "PLT-123")
			if calls != 2 {
				t.Fatalf("expected fresh lookup and statuses, got %d calls", calls)
			}
			if statusType == "" {
				if err == nil {
					t.Fatal("expected unresolved status error")
				}
				return
			}
			if err != nil || issue.Status.Type != statusType {
				t.Fatalf("unexpected issue: %+v, %v", issue, err)
			}
		})
	}
}
