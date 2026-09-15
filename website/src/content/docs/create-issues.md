---
title: Create issues
description: Create issues interactively or from explicit flags.
---

## Interactive form

```sh
lnr issue create
# short alias
lnr ic
```

The form supports a title, description, team defaults, project, labels, estimate, assignee, and status.

## Non-interactive creation

```sh
lnr issue create \
  --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently." \
  --project "Release readiness"
```

`--project` accepts a project name or ID and verifies that the project belongs to the selected team.

## Quick creation

Use [`lnr quick`](/docs/quick/) when saved defaults contain everything the issue needs.
