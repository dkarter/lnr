---
title: Automation and JSON
description: Use lnr predictably from scripts and coding agents.
---

Commands print a branch name by default, making command substitution simple:

```sh
branch=$(lnr quick "Fix flaky deployment check")
git checkout -b "$branch"
```

Use `--json` for structured output:

```sh
lnr quick --json "Fix flaky deployment check"
lnr issue search --json "deployment check"
lnr issue update PLT-123 --status Done --json
```

Issue JSON includes `issueId`, `branchName`, `title`, and `url`. For unattended creation and updates, supply all required input through flags; deletion additionally requires `--force`.

`issue search` always opens an interactive picker, even with search text and `--json`. Search text prefills the editable input; a user must press Enter to confirm an issue. The picker renders to stderr so tools can capture JSON from stdout while forwarding stdin and stderr to the terminal. Cancellation writes no result to stdout. Do not use search in unattended automation.
