---
title: Find issues
description: Search recent issues interactively or retrieve the best match.
---

## Interactive search

```sh
lnr issue search
# short alias
lnr is
```

The picker fetches a small first page immediately. As you type, lnr debounces input and asks Linear for filtered results. Move through the list to load later pages without waiting for the entire team's history.

## Non-interactive search

Pass a query to print the best matching issue's branch name:

```sh
lnr issue search "deployment check"
lnr issue search --json "deployment check"
lnr is --checkout "deployment check"
```

Use explicit search text for scripts so the command never opens a picker.
