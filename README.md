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

### Basic usage:

```bash
lnr
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
lnr issue
```

Return JSON for the selected issue:

```bash
lnr issue --json
```

Search non-interactively and print/copy the best match:

```bash
lnr issue "deployment check"
lnr issue --json "deployment check"
```

Generate shell completions:

```bash
lnr completion bash
lnr completion zsh
```

Reset cached teams, labels, and defaults:

```bash
lnr reset
```

### tmux Integration

For a better experience, add a shell function to your `~/.zshrc` or `~/.bashrc`:

```bash
# LNR form in tmux popup
lnr() {
  if [[ -n "$TMUX" ]]; then
    tmux popup -w 80% -h 80% lnr
  else
    lnr
  fi
}
```

Add a keybinding to your `~/.tmux.conf`:

```tmux
bind-key "i" display-popup -E -w 80% -h 80% lnr
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
