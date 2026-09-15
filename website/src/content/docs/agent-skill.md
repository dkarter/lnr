---
title: Agent skill
description: Teach a coding agent to use lnr safely and non-interactively.
---

Print the bundled agent skill:

```sh
lnr skill
```

The skill describes safe non-interactive issue creation and branch checkout. An agent should always provide a title, use `--json` when it needs structured values, and never combine JSON, clipboard, and checkout modes.

Example:

```sh
lnr issue create --json \
  --title "Fix flaky deployment check" \
  --description "The deployment check fails intermittently."
```
