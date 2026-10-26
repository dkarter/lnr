---
title: Read, update, and delete
description: Read current issue status, change fields, or safely remove an issue.
---

## Read

Get one issue without an interactive picker:

```sh
lnr issue get PLT-123
lnr issue get PLT-123 --json
```

JSON contains `issueId`, `branchName`, `title`, `url`, and `status: {id, name, type}`.
Use `status.type` to detect `completed` and `canceled` issues regardless of custom
status names. Status is fetched fresh, not from cached defaults. This works with
API keys and saved browser-login credentials, but never starts a browser login.
If authentication is unavailable, run `lnr auth login` separately.

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
