---
name: lnr
description: Use the local lnr CLI to create Linear issues, find existing issues, and get Linear branch names. Use when an agent needs to create or look up a Linear issue from the terminal, configure lnr defaults, or authenticate lnr.
---

# lnr

Use `lnr` for focused Linear issue workflows. Prefer its JSON output for
non-interactive agent use.

## Setup

Authenticate through the browser if needed:

```bash
lnr auth login
```

Run `lnr configure` once to select the default team, labels, estimate, and
status used by quick creation and issue search. The individual configuration
commands are `set-team`, `set-labels`, `set-estimate`, and `set-status`.

## Create an Issue

Create from a title using saved defaults:

```bash
lnr quick --json "Fix flaky deployment check"
```

Create non-interactively with a description and the same configured defaults:

```bash
lnr issue create --json --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
```

The JSON object contains `issueId`, `branchName`, `title`, and `url`. Without
`--json`, creation prints and attempts to copy the Linear branch name. Run
`lnr issue create` without creation flags for the interactive workflow. The
shorter alias is `lnr ic`.

## Find an Issue

Search recent issues in the default team and return the best match:

```bash
lnr issue search --json "deployment check"
```

Run `lnr issue search --json` without a search term for an interactive picker.
The shorter `lnr is` alias accepts the same arguments. The JSON shape matches
quick creation.

## Operational Notes

- Run `lnr COMMAND --help` before guessing command syntax.
- Use `lnr reset` to clear cached API data and saved defaults.
- `LINEAR_API_KEY` takes precedence over OAuth. `LINEAR_OAUTH_ACCESS_TOKEN` can
  provide an existing OAuth token; otherwise lnr uses browser login and caches
  its token securely.
- Without `--json`, quick creation and issue lookup print the branch name and
  attempt to copy it to the clipboard.
