---
title: Install
description: Install lnr with mise or build it from source.
---

## mise

```sh
mise use -g github:dkarter/lnr
```

This installs the latest stable GitHub release for your platform.

## From source

Clone the repository, then run:

```sh
mise run install
```

The task builds `lnr` and installs it at `~/.local/bin/lnr`.

## Verify

```sh
lnr --version
lnr --help
```
