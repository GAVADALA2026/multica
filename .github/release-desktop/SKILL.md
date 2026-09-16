---
name: release-desktop
description: Build, sign, notarize, and publish a Multica Desktop release for macOS Apple Silicon (arm64) and Intel (x64) to an existing GitHub Release created by the CLI release flow. Uploads both architectures' DMG, ZIP, blockmaps, and architecture-specific update metadata so electron-updater works on installed clients. Use after pushing a vX.Y.Z tag has triggered the `Release` GitHub Actions workflow and the CLI artifacts are published on the matching GitHub Release.
---

# release-desktop

Completes a Multica Desktop release for macOS Apple Silicon (arm64) and Intel (x64). The CLI release is already published (via `.github/workflows/release.yml` → GoReleaser). This skill adds both Desktop architectures to the same GitHub Release, so:

- Users downloading from the release page get the notarized DMG for their Mac architecture.
- Existing Apple Silicon installations update from `latest-mac.yml`.
- Existing Intel installations update from the isolated `latest-x64-mac.yml` channel.

Everything runs locally on the user's Mac so Apple credentials never leave the machine.

macOS Desktop packaging is slow by nature: code signing, notarization, stapling, and artifact upload can take many minutes with long stretches of minimal output. Treat that as normal. Do not interrupt the foreground build just because it looks quiet for a while.

## When to invoke

The user asks to "release desktop", "publish desktop", "ship desktop for `<tag>`", or passes a tag like `release-desktop v0.1.36`. Requires:

- A version tag has been pushed (e.g., `git push origin v0.1.36`).
- The GitHub Release for that tag exists with CLI artifacts attached. The rest of the `Release` GitHub Actions workflow may still be building Windows/Linux Desktop artifacts.
- User is on the designated release host — the Mac with the Apple Developer ID signing cert and notarization credentials installed. (There is typically only one such host per org.)

If any prerequisite is missing, **stop and tell the user** — don't try to work around it.

## Hard prerequisites (verify before starting)

- **Repo path**: the `multica` repo. Resolve it from the `MULTICA_REPO` env var, falling back to the current working directory if it already looks like the repo (i.e., contains `apps/desktop/electron-builder.yml`). If neither is set/valid, bail and ask the user to either `cd` into the repo or export `MULTICA_REPO=/path/to/multica`.
- **Signing cert**: `security find-identity -v -p codesigning` must list a `Developer ID Application: <Org Name> (<TeamID>)` entry whose Team ID matches `APPLE_TEAM_ID` from the credentials file (see next item). If missing, stop — the cert is not installable via this skill.
- **Apple credentials**: a shell file exporting `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID`, and (optionally) `APPLE_SIGNING_IDENTITY` (the full `Developer ID Application: ...` string used by electron-builder's `CSC_NAME`). Default location is `~/.multica/release.env` (owner-only, `chmod 600`). Override with the `MULTICA_RELEASE_ENV` environment variable if the file lives elsewhere. Source the file with `set -a && source "$MULTICA_RELEASE_ENV" && set +a` so the exports land in the current shell. If the file doesn't exist, tell the user to create it using the template in `## Setup` below — don't try to find one somewhere else on disk.
- **gh auth**: `gh auth status --active --hostname github.com` must show an authenticated active account with `repo` scope. Note: electron-builder does **not** read gh's keychain — it only reads the `GH_TOKEN` (or `GITHUB_TOKEN`) environment variable, per its [publish docs](https://www.electron.build/publish.html). Step 3 below exports `GH_TOKEN=$(gh auth token --hostname github.com)` so the same active gh-stored token gets surfaced as an env var for electron-builder.
- **Toolchain**: `go`, `node`, `pnpm` must be on PATH. Minimum versions match what `bundle-cli.mjs` and `electron-builder` expect.
- **Working tree**: `git status --porcelain` must be empty. **Never auto-stash the user's work** — bail with a clear message asking them to commit or stash first.

## Setup (one-time, per release host)

On a fresh Mac that will be used for Desktop releases, create the credentials file before the first run:

```bash
mkdir -p ~/.multica
cat > ~/.multica/release.env <<'EOF'
export APPLE_ID="<apple-id-email>"
export APPLE_APP_SPECIFIC_PASSWORD="<app-specific-password-from-appleid.apple.com>"
export APPLE_TEAM_ID="<10-char-team-id-from-developer-account>"
# Optional: pin the exact signing identity if multiple Developer ID certs are in the keychain.
# export APPLE_SIGNING_IDENTITY="Developer ID Application: <Org Name> (<TEAMID>)"
EOF
chmod 600 ~/.multica/release.env
```

The app-specific password is minted at https://appleid.apple.com → Sign-In and Security → App-Specific Passwords. It's distinct from the Apple ID login password and can be revoked independently. `APPLE_TEAM_ID` must match the Team ID on the Developer ID Application cert installed in the login keychain (find it via `security find-identity -v -p codesigning` or in the Apple Developer account membership page).

Alternate location: set `MULTICA_RELEASE_ENV=/some/other/path/release.env` before invoking the skill to point at a different file.

## Arguments

- `$1` (or the first word the user says that looks like a tag): the tag to release, e.g., `v0.1.36`. Always include the leading `v`. If missing, ask the user instead of guessing. Set the shell's `TAG` variable to this exact value before running the preflight commands.

## Steps

### 1. Preflight

```bash
TAG="${TAG:?set TAG to the exact requested vX.Y.Z tag}"
REPO="${MULTICA_REPO:-$PWD}"
[ -f "$REPO/apps/desktop/electron-builder.yml" ] || { echo "not a multica repo: $REPO — set MULTICA_REPO or cd into the repo"; exit 1; }
cd "$REPO"
gh auth status --active --hostname github.com   # confirm the active release account is authenticated
git fetch --tags origin
git rev-parse "$TAG^{commit}"   # bail if tag doesn't exist locally
gh release view "$TAG" --json tagName,assets   # bail if the release doesn't exist yet
```

Scan the existing release assets. If any macOS Desktop asset for the target version or either update metadata file (`latest-mac.yml` or `latest-x64-mac.yml`) is already attached, **ask the user** whether to re-upload (it usually means a previous attempt half-succeeded). If yes, enumerate and delete only that version's stale macOS Desktop assets and the two macOS metadata files with `gh release delete-asset "$TAG" <asset-name> --yes` before building, so electron-builder's publish step doesn't hit a 422 duplicate error. Never delete CLI, Windows, or Linux assets.

### 2. Checkout the tag

```bash
ORIGINAL_REF=$(git symbolic-ref --short HEAD 2>/dev/null || git rev-parse HEAD)
git status --porcelain | grep -q . && { echo "working tree dirty, aborting"; exit 1; }
git checkout "$TAG"
```

Track `$ORIGINAL_REF` so the final step can return the user to their original branch no matter what happens.

### 3. Environment

```bash
RELEASE_ENV="${MULTICA_RELEASE_ENV:-$HOME/.multica/release.env}"
[ -f "$RELEASE_ENV" ] || { echo "missing $RELEASE_ENV — see Setup section"; exit 1; }
set -a
source "$RELEASE_ENV"
set +a
[ -n "$APPLE_ID" ] && [ -n "$APPLE_APP_SPECIFIC_PASSWORD" ] && [ -n "$APPLE_TEAM_ID" ]   # sanity

# electron-builder ONLY reads GH_TOKEN/GITHUB_TOKEN from env — it does not read
# gh's keychain. Surface the same active gh token as an env var so the publish step works.
export GH_TOKEN=$(gh auth token --hostname github.com)
[ -n "$GH_TOKEN" ] || { echo "gh auth token returned empty — run 'gh auth login' first"; exit 1; }
```

### 4. Install + build + sign + notarize + publish

```bash
pnpm install --frozen-lockfile
pnpm --filter @multica/desktop package -- --mac --arm64 --x64 --publish always
```

What `electron-builder` does under the hood when this runs:

1. Calls `scripts/package.mjs`, derives the Desktop version from `git describe`, and creates a build matrix for darwin/arm64 and darwin/x64.
2. For each architecture, calls `bundle-cli.mjs` to build the matching `multica` CLI into `resources/bin/`, then invokes `electron-builder` with `-c.extraMetadata.version=<derived-version>` and an isolated output directory (`dist/mac-arm64` or `dist/mac-x64`).
3. Keeps the established Apple Silicon update feed on `latest-mac.yml`. For Intel, applies `mac.minimumSystemVersion=12.0.0` and publishes the isolated `latest-x64-mac.yml` feed; the runtime updater selects that channel on darwin/x64.
4. Codesigns both apps with Developer ID Application from the keychain.
5. Notarizes each build in-flight via `notarytool` using the sourced Apple credentials.
6. Staples each notarization ticket onto the `.app` **before** zipping, so the architecture-specific update metadata's SHA512 values match the published files exactly. (If you skip in-build notarization and try to re-staple afterward, the hashes drift and `electron-updater` rejects the update as corrupt — that's why `notarize: true` is set in `electron-builder.yml`.)
7. `--publish always` makes electron-builder find the matching GitHub Release (by version) and upload both architectures' macOS artifacts to it.

**Run this command in the foreground** so you can watch for errors. Both architectures are packaged and notarized sequentially, so the whole build is typically ~10–30 minutes, and slower runs are possible. Expect long quiet periods during signing/notarization/upload.

**Agent rule:** once `pnpm --filter @multica/desktop package -- --mac --arm64 --x64 --publish always` is running, wait patiently for it to exit on its own. Do not kill it, do not restart it, and do not assume it is hung just because there is no fresh output for several minutes. Only stop early for a clear, terminal error message or if the user explicitly tells you to abort.

If the build fails, show the last ~80 lines of output, record which architecture failed, and continue to Step 6 to restore the original ref. A partial run may already have uploaded one architecture; on retry, repeat Step 1 and remove only the stale macOS assets after user confirmation. Common failures:

- `build/entitlements.mac.plist: cannot read entitlement data` — the tag predates the entitlements fix in `apps/desktop/build/`. Cherry-pick the fix onto this tag, or pick a newer tag where the file already exists.
- `Could not resolve host: upload.notarytool.apple.com` — network hiccup, retry.
- Notarization rejected (`status: Invalid`) — fetch the log with `xcrun notarytool log <submission-id> --apple-id "$APPLE_ID" --password "$APPLE_APP_SPECIFIC_PASSWORD" --team-id "$APPLE_TEAM_ID"` and show the user.
- `Resource not accessible by integration` / `401 Unauthorized` / `404 Not Found` on upload — `GH_TOKEN` is unset, expired, or lacks `repo` scope. Re-run `export GH_TOKEN=$(gh auth token --hostname github.com)` in the same shell that runs `pnpm package` (env vars don't cross shells), and re-check `gh auth status --active --hostname github.com`.
- `422 already_exists` for an asset — stale artifacts on the release, delete them and retry (see step 1).

### 5. Verify the release

```bash
gh release view "$TAG" --json assets --jq '.assets[].name' | sort
```

Confirm **all** of these are present — the ten-file set is required for direct downloads and architecture-correct electron-updater auto-update:

| File | Purpose |
|---|---|
| `multica-desktop-<version>-mac-arm64.dmg` | Apple Silicon download for manual installs |
| `multica-desktop-<version>-mac-arm64.dmg.blockmap` | Apple Silicon differential DMG updates |
| `multica-desktop-<version>-mac-arm64.zip` | Apple Silicon archive downloaded by electron-updater |
| `multica-desktop-<version>-mac-arm64.zip.blockmap` | Apple Silicon differential ZIP updates |
| `latest-mac.yml` | **Critical** — Apple Silicon update metadata |
| `multica-desktop-<version>-mac-x64.dmg` | Intel download for manual installs (macOS 12+) |
| `multica-desktop-<version>-mac-x64.dmg.blockmap` | Intel differential DMG updates |
| `multica-desktop-<version>-mac-x64.zip` | Intel archive downloaded by electron-updater |
| `multica-desktop-<version>-mac-x64.zip.blockmap` | Intel differential ZIP updates |
| `latest-x64-mac.yml` | **Critical** — Intel update metadata selected by darwin/x64 clients |

`<version>` is the tag with the `v` prefix stripped (e.g., `0.1.36`) — `scripts/package.mjs` derives this automatically from `git describe`.

If any file is missing, **stop and report**. Do not try to upload individual files with `gh release upload` by hand — electron-builder generates each update metadata file with the exact SHA512s of the artifacts it uploaded, so hand-uploading a different file pair desyncs the metadata.

### 6. Cleanup

```bash
git checkout "$ORIGINAL_REF"
```

Always do this, even on failure, so the user doesn't wake up on a detached HEAD at the tag.

### 7. Report

Tell the user:
- The tag + commit that was built.
- The derived Desktop version (from `package.mjs` output).
- Confirmation that both macOS arm64 and x64 passed verification, plus the list of assets now on the release.
- The release URL: `gh release view "$TAG" --json url --jq .url`.
- That Apple Silicon clients use `latest-mac.yml` and Intel clients use `latest-x64-mac.yml` on their next updater poll.

## Safety rails

- **Never force-push, never create the GitHub Release**. The CLI release flow already creates the Release; this skill only *adds* assets to it.
- **Never commit the edited `package.json` or any file**. `scripts/package.mjs` injects the version via `-c.extraMetadata.version=`, which is a build-time override that doesn't touch the tracked file.
- **Never rm -rf `dist/`** from outside `apps/desktop/`. Stay scoped.
- **Never invoke `gh auth` flows** or switch accounts without asking. If `gh auth status --active --hostname github.com` shows the wrong account, tell the user and stop.
- **Never upload .env files or certificates** to the release.
- **Never touch the user's main branch** or create new tags — this skill is a read-only consumer of tags.
- **Never terminate the desktop packaging command early because of slow signing/notarization.** Quiet output for several minutes is expected on macOS release builds.

## Known gaps (not handled by this skill)

- **Only macOS arm64 and x64.** Linux and Windows are out of scope; the GitHub Actions release workflow handles those platforms.
- **No rollback.** If a bad version ships, the user has to manually delete assets via `gh release delete-asset` and retry with a new tag.
- **No auto-trigger.** The skill is user-invoked. A future step could wire it to a self-hosted GitHub Actions runner on the release host so the release workflow does both CLI and Desktop atomically on tag push.
- **No staging/rehearsal mode.** There's no `--dry-run` that builds without publishing. If you need to test, build locally with `pnpm --filter @multica/desktop package -- --mac --arm64 --x64` (no `--publish`) and inspect `apps/desktop/dist/mac-arm64/` and `apps/desktop/dist/mac-x64/`.

## Related files

- `apps/desktop/electron-builder.yml` — config (signing, notarize, publish target, artifact names).
- `apps/desktop/build/entitlements.mac.plist` — hardened-runtime entitlements (JIT, disable-library-validation for spawning the Go CLI, network client/server).
- `apps/desktop/scripts/package.mjs` — version-sync wrapper around electron-builder.
- `apps/desktop/scripts/bundle-cli.mjs` — builds the `multica` Go binary and copies it into `resources/bin/` so the packaged app ships with a matching CLI.
- `.github/workflows/release.yml` + `.goreleaser.yml` — upstream CLI release flow this skill plugs into.
