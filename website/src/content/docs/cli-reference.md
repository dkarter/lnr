---
title: CLI reference
description: Command map and common flags for lnr.
---

| Command | Purpose |
| --- | --- |
| `lnr quick [TITLE]` | Create with saved defaults |
| `lnr issue create` | Create interactively or with flags |
| `lnr issue search [SEARCH]` | Pick an issue or print the best match |
| `lnr issue update ISSUE` | Update title, description, team, status, or project |
| `lnr issue delete ISSUE` | Delete with confirmation |
| `lnr auth login` | Start browser OAuth |
| `lnr auth logout` | Clear authentication and account state |
| `lnr config` | Configure defaults |
| `lnr reset` | Clear cache and defaults |
| `lnr skill` | Print the agent skill |
| `lnr completion SHELL` | Generate shell completion output |

Run `lnr COMMAND --help` for the exact flags supported by your installed version.

## Common output flags

- `--json` prints structured output.
- `--copy` copies the generated branch name.
- `--checkout`, `-c` creates and checks out the generated branch.

These output modes are mutually exclusive.
