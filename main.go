package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
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

func getCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "lnr")
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

	if time.Since(entry.Timestamp) > ttl {
		return nil, false
	}

	return entry.Data, true
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

func main() {
	// Parse command-line flags
	clearCacheFlag := flag.Bool("clear-cache", false, "Clear the cache and refetch all data")
	flag.Parse()

	// Handle clear cache flag
	if *clearCacheFlag {
		if err := clearCache(); err != nil {
			fmt.Printf("❌ Error clearing cache: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Cache cleared successfully")
		return
	}

	var ticket LinearTicket
	var selections UserSelections

	// Get API credentials
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ LINEAR_API_KEY environment variable not set")
		fmt.Println("Set this to create tickets in Linear")
		fmt.Println("\nExample:")
		fmt.Println("  export LINEAR_API_KEY='your-api-key'")
		os.Exit(1)
	}

	// Load cached selections
	cacheTTL := 24 * time.Hour
	if cachedSelections, found := loadFromCache("user-selections", cacheTTL); found {
		// Convert cached data back to UserSelections
		cachedData := cachedSelections.(map[string]interface{})
		selections = UserSelections{
			TeamId:     getString(cachedData, "teamId"),
			AssigneeId: getString(cachedData, "assigneeId"),
			Estimate:   getString(cachedData, "estimate"),
			StatusId:   getString(cachedData, "statusId"),
		}
		if labels, ok := cachedData["labels"].([]interface{}); ok {
			for _, label := range labels {
				selections.Labels = append(selections.Labels, label.(string))
			}
		}
	}

	// Fetch teams
	var teams []Team
	var err error
	if cachedTeams, found := loadFromCache("teams", cacheTTL); found {
		// Convert cached data back to []Team
		cachedData := cachedTeams.([]interface{})
		teams = make([]Team, len(cachedData))
		for i, item := range cachedData {
			itemMap := item.(map[string]interface{})
			teams[i] = Team{
				ID:   itemMap["id"].(string),
				Name: itemMap["name"].(string),
			}
		}
	} else {
		teams, err = fetchTeams(apiKey)
		if err != nil {
			fmt.Printf("❌ Error fetching teams: %v\n", err)
			os.Exit(1)
		}
		saveToCache("teams", teams)
	}

	// Create team selection options
	teamOptions := make([]huh.Option[string], len(teams))
	for i, team := range teams {
		teamOptions[i] = huh.Option[string]{Key: team.Name, Value: team.ID}
	}

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

	if cachedLabels, found := loadFromCache("labels-"+selectedTeamId, cacheTTL); found {
		// Convert cached data back to []Label
		cachedData := cachedLabels.([]interface{})
		labels = make([]Label, len(cachedData))
		for i, item := range cachedData {
			itemMap := item.(map[string]interface{})
			labels[i] = Label{
				ID:   itemMap["id"].(string),
				Name: itemMap["name"].(string),
			}
		}
	} else {
		labels, err = fetchTeamLabels(apiKey, selectedTeamId)
		if err != nil {
			fmt.Printf("❌ Error fetching labels: %v\n", err)
			os.Exit(1)
		}
		saveToCache("labels-"+selectedTeamId, labels)
	}

	if cachedUsers, found := loadFromCache("users-"+selectedTeamId, cacheTTL); found {
		// Convert cached data back to []User
		cachedData := cachedUsers.([]interface{})
		users = make([]User, len(cachedData))
		for i, item := range cachedData {
			itemMap := item.(map[string]interface{})
			users[i] = User{
				ID:    itemMap["id"].(string),
				Name:  itemMap["name"].(string),
				Email: itemMap["email"].(string),
			}
		}
	} else {
		users, err = fetchTeamUsers(apiKey, selectedTeamId)
		if err != nil {
			fmt.Printf("❌ Error fetching users: %v\n", err)
			os.Exit(1)
		}
		saveToCache("users-"+selectedTeamId, users)
	}

	if cachedStates, found := loadFromCache("states-"+selectedTeamId, cacheTTL); found {
		// Convert cached data back to []WorkflowState
		cachedData := cachedStates.([]interface{})
		workflowStates = make([]WorkflowState, len(cachedData))
		for i, item := range cachedData {
			itemMap := item.(map[string]interface{})
			workflowStates[i] = WorkflowState{
				ID:   itemMap["id"].(string),
				Name: itemMap["name"].(string),
				Type: getString(itemMap, "type"),
			}
		}
	} else {
		workflowStates, err = fetchWorkflowStates(apiKey, selectedTeamId)
		if err != nil {
			fmt.Printf("❌ Error fetching workflow states: %v\n", err)
			os.Exit(1)
		}
		saveToCache("states-"+selectedTeamId, workflowStates)
	}

	// Create options
	estimateOptions := getEstimateOptions(1) // Default to story points

	labelOptions := make([]huh.Option[string], len(labels))
	labelMap := make(map[string]string)
	for i, label := range labels {
		labelOptions[i] = huh.Option[string]{Key: label.Name, Value: label.Name}
		labelMap[label.Name] = label.ID
	}

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

	// Create the ticket in Linear
	if apiKey == "" {
		fmt.Println("\n⚠️  LINEAR_API_KEY environment variable not set")
		fmt.Println("Set this to automatically create tickets in Linear")
		fmt.Println("\nExample:")
		fmt.Println("  export LINEAR_API_KEY='your-api-key'")
		return
	}

	fmt.Println("\n🚀 Creating ticket in Linear...")
	issueId, err := createLinearTicket(apiKey, ticket, labelMap)
	if err != nil {
		fmt.Printf("❌ Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Ticket created successfully! ID: %s\n", issueId)

	// Save user selections to cache
	selections = UserSelections{
		TeamId:     ticket.TeamId,
		AssigneeId: ticket.AssigneeId,
		Labels:     ticket.Labels,
		Estimate:   ticket.Estimate,
		StatusId:   ticket.StatusId,
	}
	saveToCache("user-selections", selections)

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
		branchName := strings.ToLower(issueId)
		if err := clipboard.WriteAll(branchName); err != nil {
			fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
		} else {
			fmt.Printf("📋 Copied '%s' to clipboard\n", branchName)
		}
	case "open":
		// Get the full URL from the issue data
		url := fmt.Sprintf("https://linear.app/issue/%s", issueId)
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

func createLinearTicket(apiKey string, ticket LinearTicket, labelMap map[string]string) (string, error) {
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
