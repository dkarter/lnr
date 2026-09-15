---
title: Configuration
description: Configure the defaults that make lnr quick.
---

Run the complete setup:

```sh
lnr config
```

Or update one default at a time:

```sh
lnr config set-team
lnr config set-labels
lnr config set-estimate
lnr config set-status
```

Defaults are stored under your XDG configuration directory, normally `~/.config/lnr/defaults.json`. Cached Linear metadata is stored under your XDG cache directory, normally `~/.cache/lnr`.

Clear cached metadata and defaults with:

```sh
lnr reset
```
