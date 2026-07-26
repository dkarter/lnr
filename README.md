# Linear Ticket Form

A beautiful TUI form for creating Linear tickets from the command line, perfect for Herdr or tmux popup panes.

## Features

- 🏎️ Designed for speed and reducing friction
- 🎯 Story point estimation
- 🏷️ Label selection
- 📝 Full description support
- 🚀 Automatic ticket creation via Linear API
- 🔐 Browser-based OAuth sign-in with Dynamic Client Registration
- ✨ Interactive form with text input, textarea, dropdown, and multi-select
- 💻 Interactive TUI is Perfect for herdr/tmux popups
- 🤖 Perfect for agents! tell your agent to install the skill from `lnr skill`
- 🤏 Minimal output to reduce context usage

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

### OAuth

Start the OAuth flow explicitly:

```bash
lnr auth login
```

Or just run any command that needs Linear access. `lnr` will open your browser, ask you to approve access in Linear, and store the token at `~/.cache/lnr/oauth-token.json` with `0600` permissions.

Clear the saved OAuth token, cached API data, and account-specific defaults:

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

### Token authentication

<details>
<summary>Personal API key setup</summary>

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

</details>

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

Create non-interactively with configured defaults. This prints the Linear git
branch name by default:

```bash
lnr issue create --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
lnr ic --json --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
```

### Quick usage:

Configure the defaults used by quick commands:

```bash
lnr config
```

Or set defaults individually:

```bash
lnr config set-team
lnr config set-labels
lnr config set-estimate
lnr config set-status
```

Create an issue from only a title and print its Linear git branch name:

```bash
lnr quick
lnr quick "Fix flaky deployment check"
lnr --quick "Fix flaky deployment check"
```

When the title is omitted, `lnr quick` prompts for it interactively.

Create the issue and immediately check out its Linear git branch:

```bash
lnr quick --checkout "Fix flaky deployment check"
lnr quick -c
lnr issue create -c --title "Fix flaky deployment check"
```

Copy a branch name to the clipboard only when requested:

```bash
lnr quick --copy "Fix flaky deployment check"
lnr issue search --copy "deployment check"
```

Return JSON:

```bash
lnr quick --json "Fix flaky deployment check"
```

Fuzzy find a recent issue in the default team and print its branch name:

```bash
lnr issue search
```

Return JSON for the selected issue:

```bash
lnr issue search --json
```

Find an issue and check out its Linear git branch:

```bash
lnr issue search --checkout "deployment check"
lnr is -c "deployment check"
```

Search non-interactively and print the best match:

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

### Herdr Integration

Add a custom popup command to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+i"
type = "popup"
command = "lnr issue create"
width = "80%"
height = "80%"
```

Reload Herdr's configuration:

```bash
herdr server reload-config
```

Then press `prefix + i` to open the issue creation workflow in a popup.

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
