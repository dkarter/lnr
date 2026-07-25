---
name: lnr
description: Use the local lnr CLI non-interactively to create Linear tickets and work with their branches.
---

# lnr

Use `lnr` to create Linear tickets and obtain or check out their branches.
Never run a command that can prompt for terminal input.

Create from a title using saved defaults:

```bash
lnr quick --json "Fix flaky deployment check"
```

The JSON result contains `issueId`, `branchName`, `title`, and `url`.

Create with a description:

```bash
lnr issue create --json --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
```

Create a ticket and immediately check out its Linear branch:

```bash
lnr quick --checkout "Fix flaky deployment check"
# Short form:
lnr quick -c "Fix flaky deployment check"
```

Always provide `--title` to `issue create` and a title argument to `quick`.
Never combine `--json` with `--checkout`.
