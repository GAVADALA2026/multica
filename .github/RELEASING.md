# Release runbook

## Normal release

Release from a reviewed commit on `main` by creating and pushing a new semantic
version tag such as `v0.18.4`. The Release workflow intentionally has no manual
trigger: a tag push is the only event that can publish binaries, Homebrew
formulae, and container images.

The macOS Desktop artifacts are still signed and notarized on the designated
release Mac. Use the repository's
[`release-desktop`](release-desktop/SKILL.md) runbook after the matching GitHub
Release and CLI assets exist.

The workspace `Release Desktop` skill is the executable copy of this runbook;
repository files are not registered as workspace skills automatically. After a
change to the runbook merges, its creator or a workspace owner/admin must check
out the merged `main` branch and synchronize it before the next macOS release:

```bash
SKILL_ID=$(multica skill list --output json | jq -r \
  '[.[] | select(.name == "Release Desktop")] | if length == 1 then .[0].id else error("expected exactly one Release Desktop skill") end')
multica skill update "$SKILL_ID" \
  --content-file .github/release-desktop/SKILL.md \
  --output json
multica skill get "$SKILL_ID" --output json
shasum -a 256 .github/release-desktop/SKILL.md
```

Updating the existing skill preserves its ID and agent bindings. Confirm the
returned `content_hash` changed to the merged runbook's hash before releasing.

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
