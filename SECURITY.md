# Security

Pigeonhole holds the logins to people's messaging accounts and updates itself
from this repository's releases, so security problems matter. Thanks for
reporting them.

## Reporting a problem

Please **don't open a public issue** for anything that could put users at
risk. Instead, report it privately through GitHub:
[Security → Report a vulnerability](https://github.com/henzyliu/pigeonhole/security/advisories/new).

Include what you found, how to reproduce it, and which version you saw it in
(Settings → About shows the version and build). This is a personal project
maintained in spare time, so replies may take a few days, but every report
is read.

## Supported versions

Only the [latest release](https://github.com/henzyliu/pigeonhole/releases/latest)
gets fixes. Pigeonhole updates itself, so most people are on it.

## What's in scope

- Pigeonhole's own code: the app, its local storage, the login window, and
  the updater (which only installs downloads that match the release's
  `SHA256SUMS.txt`).
- How releases are built and published (`.github/workflows/release.yml`).

Problems in the messaging bridges it's built on are best reported to the
[mautrix](https://github.com/mautrix) projects directly, as are problems with
the networks themselves to Meta, WhatsApp or Discord.
