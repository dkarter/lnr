# Linear Ticket Form

A beautiful TUI form for creating Linear tickets from the command line, perfect for tmux popup panes.

## Features

- ✨ Interactive form with text input, textarea, dropdown, and multi-select
- 🎯 Story point estimation
- 🏷️ Label selection
- 📝 Full description support
- 🚀 Automatic ticket creation via Linear API
- 🔐 Browser-based OAuth sign-in with Dynamic Client Registration
- 💻 Perfect for tmux popups

## Installation

### Using mise (Recommended)

```bash
mise use -g github:dkarter/lnr
```

### From Source

I have created a simple mise task to build and install from source:

```bash
mise run install
```

## Configuration

By default, `lnr` signs in with Linear OAuth using Dynamic Client Registration. You do not need to create a Linear OAuth app manually.

Start the OAuth flow explicitly:

```bash
lnr auth login
```

Or just run any command that needs Linear access. `lnr` will open your browser, ask you to approve access in Linear, and store the token at `~/.cache/lnr/oauth-token.json` with `0600` permissions.

Clear the saved OAuth token:

```bash
lnr auth logout
```

You can customize OAuth scopes if needed:

```bash
export LINEAR_OAUTH_SCOPES='read write'
```

You can also bypass browser login with an existing OAuth access token:

```bash
export LINEAR_OAUTH_ACCESS_TOKEN='your-oauth-access-token'
```

Personal API keys are still supported and take precedence over OAuth:

1. **Get your Linear API Key:**
   - Go to Linear Settings → API → Personal API keys
   - Create a new key

2. **Set the environment variable:**

```bash
export LINEAR_API_KEY='lin_api_xxxxxxxxxxxxxxxxxx'
```

Add these to your `~/.bashrc.local` or `~/.zshrc.local` to make them available in your shell and restart your shell.

And add

```bash
source ~/.zshrc.local
```

or

```bash
source ~/.bashrc.local
```

> [!WARNING]
> Important! Never commit your .local files since they may contain sensitive
> information.

## Usage

### Command help:

```bash
lnr
```

Create an issue with the interactive workflow:

```bash
lnr issue create
lnr ic # Short alias
```

Create non-interactively with configured defaults. This prints and copies the
Linear branch name by default:

```bash
lnr issue create --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
lnr ic --json --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
```

### Quick usage:

Configure the defaults used by quick commands:

```bash
lnr configure
```

Or set defaults individually:

```bash
lnr set-team
lnr set-labels
lnr set-estimate
lnr set-status
```

Create an issue from only a title and print/copy Linear's branch name:

```bash
lnr quick "Fix flaky deployment check"
lnr --quick "Fix flaky deployment check"
```

Return JSON instead of copying the branch name:

```bash
lnr quick --json "Fix flaky deployment check"
```

Fuzzy find a recent issue in the default team and print/copy its branch name:

```bash
lnr issue search
```

Return JSON for the selected issue:

```bash
lnr issue search --json
```

Search non-interactively and print/copy the best match:

```bash
lnr issue search "deployment check"
lnr issue search --json "deployment check"

# Short alias
lnr is --json "deployment check"
```

Generate shell completions:

```bash
lnr completion bash
lnr completion zsh
lnr completion fish
lnr completion powershell
```

Reset cached teams, labels, and defaults:

```bash
lnr reset
```

### tmux Integration

For a better experience, add a shell function to your `~/.zshrc` or `~/.bashrc`:

```bash
# LNR issue creation in tmux popup
lnr-create() {
  if [[ -n "$TMUX" ]]; then
    tmux popup -w 80% -h 80% 'lnr issue create'
  else
    command lnr issue create
  fi
}
```

Add a keybinding to your `~/.tmux.conf`:

```tmux
bind-key "i" display-popup -E -w 80% -h 80% 'lnr issue create'
```

Then press `prefix + i` to open the form in a popup!

### Ghostty Integration (Optional)

Map a key in Ghostty to launch the tmux shortcut in your Ghostty config:

```ini
# Create a new linear issue
keybind = super+shift+i=text:\x1ai
```

## License

MIT
