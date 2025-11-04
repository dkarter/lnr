# Linear Ticket Form

A beautiful TUI form for creating Linear tickets from the command line, perfect for tmux popup panes.

## Features

- ✨ Interactive form with text input, textarea, dropdown, and multi-select
- 🎯 Story point estimation
- 🏷️ Label selection
- 📝 Full description support
- 🚀 Automatic ticket creation via Linear API
- 💻 Perfect for tmux popups

## Installation

```bash
# Clone or copy this directory
cd linear-ticket-form

# Install dependencies
go get github.com/charmbracelet/huh

# Build the binary
go build -o linear-ticket

# Optionally, move to your PATH
sudo mv linear-ticket /usr/local/bin/
```

## Configuration

You'll need a Linear API key and your team ID:

1. **Get your Linear API Key:**
   - Go to Linear Settings → API → Personal API keys
   - Create a new key

2. **Get your Team ID:**
   - Go to your team in Linear
   - The team ID is in the URL: `linear.app/team/TEAM_ID/...`
   - Or use the Linear API to list teams

3. **Set environment variables:**

```bash
export LINEAR_API_KEY='lin_api_xxxxxxxxxxxxxxxxxx'
export LINEAR_TEAM_ID='your-team-uuid'
```

Add these to your `~/.bashrc` or `~/.zshrc` to make them permanent.

## Usage

### Basic usage:
```bash
linear-ticket
```

### In a tmux popup:
```bash
# Add this to your tmux.conf for a keybinding
bind-key T display-popup -E -w 80% -h 80% "linear-ticket"
```

Then press `prefix + Shift+T` to open the form in a popup!

### Alternative: Create a shell script wrapper

Create `~/bin/new-ticket`:
```bash
#!/bin/bash
linear-ticket
```

Then in tmux:
```bash
bind-key T display-popup -E -w 80% -h 80% "new-ticket"
```

## Customization

### Modify Estimates

Edit the `estimateOptions` in `main.go` to match your team's estimation scale:

```go
estimateOptions := []huh.Option[string]{
    {Key: "XS - < 1 hour", Value: "0.5"},
    {Key: "S - 1-2 hours", Value: "1"},
    {Key: "M - 1 day", Value: "3"},
    {Key: "L - 2-3 days", Value: "5"},
    {Key: "XL - 1 week", Value: "8"},
}
```

### Modify Labels

Edit the `labelOptions` to match your workspace labels:

```go
labelOptions := []huh.Option[string]{
    {Key: "bug", Value: "bug"},
    {Key: "feature", Value: "feature"},
    {Key: "tech-debt", Value: "tech-debt"},
    {Key: "design", Value: "design"},
}
```

### Adding Label IDs

To fully support labels, you'll need to map label names to IDs. You can fetch these from Linear:

```bash
curl -X POST https://api.linear.app/graphql \
  -H "Authorization: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ issueLabels { nodes { id name } } }"}'
```

Then update the `createLinearTicket` function to use the IDs.

## Form Controls

- **Text Input**: Type normally, Enter to confirm
- **Text Area**: Type multiple lines, Ctrl+J for new line, Enter to confirm
- **Select**: ↑/↓ to navigate, Enter to select
- **Multi-Select**: ↑/↓ to navigate, Space to toggle, Enter to confirm
- **Esc**: Cancel form

## Example Workflow

1. Open tmux popup with `prefix + Shift+T`
2. Fill in the form:
   - Title: "Add dark mode toggle"
   - Description: "Users want ability to switch themes"
   - Estimate: 3 points
   - Labels: feature, frontend
3. Press Enter to submit
4. Ticket created automatically in Linear! ✨

## Troubleshooting

### "Form cancelled or error"
- You pressed Esc or there was an input error
- Check that required fields are filled

### "LINEAR_API_KEY and LINEAR_TEAM_ID environment variables not set"
- Export the environment variables as shown above
- Restart your shell or source your rc file

### "Linear API error"
- Check your API key is valid
- Verify your team ID is correct
- Ensure you have permission to create issues

## Advanced: GitHub Integration

You could extend this to also create GitHub issues, or link the Linear ticket to a GitHub PR:

```go
// Add GitHub API integration
githubIssueURL := createGitHubIssue(ticket)
// Then link in Linear description
```

## License

MIT - Feel free to customize for your team!
