---
title: lnr quick
description: Create an issue from a title using saved defaults.
---

`lnr quick` is the shortest path from an idea to a tracked issue. It uses your configured team, labels, estimate, and status, then prints the Linear branch name.

```sh
lnr quick "Fix flaky deployment check"
```

If you omit the title in an interactive terminal, lnr asks for one:

```sh
lnr quick
```

## JSON output

Use `--json` when a script or coding agent needs the complete result:

```sh
lnr quick --json "Fix flaky deployment check"
```

```json
{
  "issueId": "PLT-842",
  "branchName": "plt-842-fix-flaky-deployment-check",
  "title": "Fix flaky deployment check",
  "url": "https://linear.app/example/issue/PLT-842/fix-flaky-deployment-check"
}
```

## Create and check out the branch

```sh
lnr quick --checkout "Fix flaky deployment check"
# short form
lnr quick -c "Fix flaky deployment check"
```

This validates the current Git worktree before creating the issue, then creates and checks out Linear's suggested branch.

## Copy the branch name

```sh
lnr quick --copy "Fix flaky deployment check"
```

## Output modes

| Mode | Result |
| --- | --- |
| Default | Prints the branch name |
| `--json` | Prints structured issue data |
| `--copy` | Copies the branch name |
| `--checkout`, `-c` | Creates and checks out the branch |

`--json`, `--copy`, and `--checkout` are mutually exclusive.

The legacy form `lnr --quick "Title"` still works, but `lnr quick "Title"` is preferred.
