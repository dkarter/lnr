package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/charmbracelet/huh"
)

type LinearTicket struct {
	Title       string
	Description string
	Estimate    string
	Labels      []string
}

func main() {
	var ticket LinearTicket

	// Define estimate options
	estimateOptions := []huh.Option[string]{
		{Key: "0 - No estimate", Value: "0"},
		{Key: "1 - Small (< 1 day)", Value: "1"},
		{Key: "2 - Medium (1-2 days)", Value: "2"},
		{Key: "3 - Large (3-5 days)", Value: "3"},
		{Key: "5 - Extra Large (1+ weeks)", Value: "5"},
		{Key: "8 - Epic (2+ weeks)", Value: "8"},
	}

	// Define common label options (customize these for your workspace)
	labelOptions := []huh.Option[string]{
		{Key: "bug", Value: "bug"},
		{Key: "feature", Value: "feature"},
		{Key: "improvement", Value: "improvement"},
		{Key: "documentation", Value: "documentation"},
		{Key: "urgent", Value: "urgent"},
		{Key: "backlog", Value: "backlog"},
	}

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
		),
	)

	// Run the form
	err := form.Run()
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
	fmt.Printf("Estimate:    %s points\n", ticket.Estimate)
	fmt.Printf("Labels:      %v\n", ticket.Labels)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create the ticket in Linear
	apiKey := os.Getenv("LINEAR_API_KEY")
	teamId := os.Getenv("LINEAR_TEAM_ID")

	if apiKey == "" || teamId == "" {
		fmt.Println("\n⚠️  LINEAR_API_KEY and LINEAR_TEAM_ID environment variables not set")
		fmt.Println("Set these to automatically create tickets in Linear")
		fmt.Println("\nExample:")
		fmt.Println("  export LINEAR_API_KEY='your-api-key'")
		fmt.Println("  export LINEAR_TEAM_ID='your-team-id'")
		return
	}

	fmt.Println("\n🚀 Creating ticket in Linear...")
	issueId, err := createLinearTicket(apiKey, teamId, ticket)
	if err != nil {
		fmt.Printf("❌ Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Ticket created successfully! ID: %s\n", issueId)
}

func createLinearTicket(apiKey, teamId string, ticket LinearTicket) (string, error) {
	// GraphQL mutation to create an issue
	mutation := `
		mutation IssueCreate($input: IssueCreateInput!) {
			issueCreate(input: $input) {
				success
				issue {
					id
					identifier
					title
					url
				}
			}
		}
	`

	// Prepare the input
	input := map[string]interface{}{
		"teamId":      teamId,
		"title":       ticket.Title,
		"description": ticket.Description,
	}

	// Add estimate if provided
	if ticket.Estimate != "" && ticket.Estimate != "0" {
		input["estimate"] = ticket.Estimate
	}

	// Add labels if provided (you'll need to map label names to IDs)
	if len(ticket.Labels) > 0 {
		// Note: Linear requires label IDs, not names
		// You'll need to fetch label IDs first or maintain a mapping
		fmt.Println("⚠️  Note: Label creation requires label IDs. Add label mapping in code.")
	}

	payload := map[string]interface{}{
		"query": mutation,
		"variables": map[string]interface{}{
			"input": input,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Make the API request
	req, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Check for errors
	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		return "", fmt.Errorf("Linear API error: %v", errors)
	}

	// Extract issue ID
	data := result["data"].(map[string]interface{})
	issueCreate := data["issueCreate"].(map[string]interface{})
	issue := issueCreate["issue"].(map[string]interface{})

	return issue["identifier"].(string), nil
}
