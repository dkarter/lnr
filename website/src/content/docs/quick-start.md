---
title: Quick start
description: Sign in, choose defaults, and create your first Linear issue.
---

## 1. Sign in

```sh
lnr auth login
```

lnr opens Linear in your browser and stores the resulting OAuth token with restricted file permissions.

## 2. Set defaults

```sh
lnr config
```

Choose the team, labels, estimate, and workflow status used by quick creation.

## 3. Create an issue

```sh
lnr quick "Fix flaky deployment check"
```

lnr prints the branch name by default. Add `--checkout` to create that Git branch immediately:

```sh
lnr quick --checkout "Fix flaky deployment check"
```

Open the complete interactive form when the issue needs more context:

```sh
lnr issue create
```
