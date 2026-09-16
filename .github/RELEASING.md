# Release runbook

## Normal release

Release from a reviewed commit on `main` by creating and pushing a new semantic
version tag such as `v0.18.4`. The Release workflow intentionally has no manual
trigger: a tag push is the only event that can publish binaries, Homebrew
formulae, and container images.

The verification job runs the Go tests and `govulncheck` before any publishing
job starts. The vulnerability scan is fail-closed by default.

## Emergency vulnerability-scan bypass

Use the bypass only when `govulncheck` itself or its live vulnerability database
is unavailable, or when maintainers have documented a confirmed false positive
that blocks an urgent release. Never use it to publish a release with an
unresolved reachable vulnerability.

1. Record the reason and maintainer approval in the release issue or pull
   request, and confirm no other release is in progress.
2. In **Settings → Secrets and variables → Actions → Variables**, set the
   repository variable `ALLOW_VULN_BYPASS_FOR_TAG` to the exact release tag,
   for example `v0.18.4`.
3. Re-run the failed Release workflow for that tag. A different tag, an empty
   value, or any typo keeps the scan enabled.
4. Confirm the verification log contains the explicit bypass warning and retain
   the workflow URL in the incident record.
5. Delete `ALLOW_VULN_BYPASS_FOR_TAG` immediately after the release run
   completes. The tag-scoped value prevents a concurrent release with another
   tag from inheriting the bypass.

Every Go binary retains its compiler version in the standard Go build metadata;
use `go version -m <binary>` when auditing a downloaded release artifact.

## macOS Desktop artifacts

The tag-triggered Release workflow publishes the CLI and creates the GitHub
Release, but signing and notarizing the macOS Desktop application requires a
maintainer's macOS keychain. After the Release contains both
`multica_darwin_amd64.tar.gz` and `multica_darwin_arm64.tar.gz`, run:

```bash
scripts/release-desktop.sh v0.18.4
```

The script requires `gh`, Go, Node.js, pnpm, and a matching Developer ID
Application certificate. It reads `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`,
and `APPLE_TEAM_ID` from `~/.multica/release.env` by default; set
`MULTICA_RELEASE_ENV` to use another file. The credential file must not be
accessible by group or others (`chmod 600`) and must never be committed.

The script checks out the exact tag, installs locked dependencies, delegates
the actual clean/build/sign/notarize/upload work to the Desktop package script,
verifies all ten expected macOS assets, and restores the caller's original
branch or detached commit on every exit path.

If any macOS Desktop asset already exists, the script fails closed rather than
overwriting or deleting release state. Inspect the Release first; if the
previous attempt was incomplete, explicitly delete its macOS Desktop assets
before retrying. A failed upload can therefore be recovered without silently
mixing artifacts from separate builds.
