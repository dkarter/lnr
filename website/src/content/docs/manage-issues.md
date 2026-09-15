---
title: Update and delete
description: Change issue fields or safely remove an issue.
---

## Update

Only fields supplied on the command line are changed:

```sh
lnr issue update PLT-123 --title "Updated title" --description "New details"
lnr issue update PLT-123 --team Platform --status Done --project "Release readiness"
```

Team, status, and project values may be names or IDs. lnr validates their relationships before updating Linear.

Clear a project explicitly:

```sh
lnr issue update PLT-123 --no-project
```

## Delete

```sh
lnr issue delete PLT-123
```

lnr asks for confirmation. Automation must opt in to deletion:

```sh
lnr issue delete PLT-123 --force --json
```

Deletion requires `LINEAR_API_KEY`; Linear's hosted OAuth service does not expose it.
