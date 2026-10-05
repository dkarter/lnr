---
title: Find issues
description: Search issues interactively and confirm the issue you want.
---

## Interactive search

```sh
lnr issue search
# short alias
lnr is
```

The picker fetches a small first page immediately. As you type, lnr debounces input and asks Linear for filtered results. Empty input lists the team's issues. Move through the list to load later pages without waiting for the entire team's history. Press Enter to confirm an issue, or Esc/Ctrl+C to cancel without output.

## Prefilled search

Pass a query to open the picker with editable search text. The initial request filters results using that query; it does not select or confirm an issue:

```sh
lnr issue search "deployment check"
lnr issue search --json "deployment check"
lnr is --checkout "deployment check"
```

Search always requires explicit confirmation, including with `--json`. The picker renders to stderr, and stdout contains only the selected branch name or JSON. Tools can capture stdout while forwarding stdin and stderr to the terminal. Search is not suitable for unattended scripts.
