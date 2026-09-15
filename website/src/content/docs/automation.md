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

Issue JSON includes `issueId`, `branchName`, `title`, and `url`. Supply all required input through flags in automation. Machine-readable modes do not open interactive pickers or confirmations; deletion additionally requires `--force`.
