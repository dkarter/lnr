---
title: Terminal integrations
description: Open lnr in a Herdr or tmux popup.
---

## Herdr

Add a popup command to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+i"
type = "popup"
command = "lnr issue create"
width = "80%"
height = "80%"
```

Reload Herdr with `herdr server reload-config`, then press `prefix + i`.

## tmux

```text
bind-key "i" display-popup -E -w 80% -h 80% 'lnr issue create'
```

## Shell completions

```sh
lnr completion bash
lnr completion zsh
lnr completion fish
lnr completion powershell
```
