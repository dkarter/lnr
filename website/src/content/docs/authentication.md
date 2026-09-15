---
title: Authentication
description: Use browser OAuth or a Linear personal API key.
---

## Browser OAuth

OAuth is the default. Run `lnr auth login`, or run any command that needs Linear and sign in when prompted.

```sh
lnr auth login
```

Override the requested scopes only when necessary:

```sh
export LINEAR_OAUTH_SCOPES='read write'
```

Log out and clear account-specific cached data with:

```sh
lnr auth logout
```

## Personal API key

Set `LINEAR_API_KEY` to use a personal API key. It takes precedence over OAuth.

```sh
export LINEAR_API_KEY='lin_api_...'
```

Never commit the key. Issue deletion requires a personal API key because Linear's hosted OAuth service does not expose deletion.
