#!/usr/bin/env bash
# Never trace this script: xtrace would expose credentials loaded below.
set +x
set -Eeuo pipefail

# Release the signed and notarized macOS Desktop artifacts for an existing
# GitHub Release. The tag-triggered release workflow creates the Release and
# publishes the CLI first; this script deliberately refuses to run until those
# artifacts exist, and it never deletes assets from a partially completed run.

readonly REPOSITORY="multica-ai/multica"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel)"
readonly REPO_ROOT

current_step="initialization"
checkout_changed=0
original_branch=""
original_commit=""

log() {
  printf '[release-desktop] %s\n' "$*"
}

fail() {
  printf '[release-desktop] ERROR: %s\n' "$*" >&2
  exit 1
}

step() {
  current_step=$1
  log "==> $current_step"
}

restore_checkout() {
  if [ "$checkout_changed" -eq 0 ]; then
    return 0
  fi

  log "Restoring the original checkout"
  if [ -n "$original_branch" ]; then
    git switch --quiet "$original_branch"
  else
    git switch --quiet --detach "$original_commit"
  fi
  checkout_changed=0
}

finish() {
  local status=$?
  local restore_status=0

  trap - EXIT INT TERM
  set +e
  restore_checkout
  restore_status=$?

  if [ "$restore_status" -ne 0 ]; then
    printf '[release-desktop] ERROR: failed to restore the original checkout (%s at %s)\n' \
      "${original_branch:-detached $original_commit}" "$REPO_ROOT" >&2
    status=1
  fi

  if [ "$status" -eq 0 ]; then
    log "Completed successfully"
  else
    printf '[release-desktop] FAILED during "%s" (exit %s)\n' \
      "$current_step" "$status" >&2
  fi
  exit "$status"
}

trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_asset() {
  local assets=$1
  local expected=$2

  printf '%s\n' "$assets" | grep -Fxq -- "$expected" ||
    fail "GitHub Release is missing required asset: $expected"
}

file_mode() {
  local path=$1
  local mode

  if mode=$(stat -f '%Lp' "$path" 2>/dev/null); then
    printf '%s\n' "$mode"
    return
  fi
  stat -c '%a' "$path"
}

if [ "$#" -ne 1 ]; then
  fail "usage: scripts/release-desktop.sh <vMAJOR.MINOR.PATCH[-PRERELEASE]>"
fi

readonly TAG=$1
readonly VERSION=${TAG#v}

if [[ ! "$TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
  fail "tag must be a semantic version prefixed with v (received: $TAG)"
fi

cd "$REPO_ROOT"

step "checking the release host and required tools"
[ "$(uname -s)" = "Darwin" ] || fail "macOS releases must run on a macOS host"
for command_name in git gh security go node pnpm stat grep; do
  require_command "$command_name"
done

origin_url="$(git remote get-url origin)"
readonly origin_url
case "$origin_url" in
  git@github.com:multica-ai/multica.git | \
    ssh://git@github.com/multica-ai/multica.git | \
    https://github.com/multica-ai/multica | \
    https://github.com/multica-ai/multica.git)
    ;;
  *)
    fail "origin must point to github.com/$REPOSITORY (received: $origin_url)"
    ;;
esac

if [ -n "$(git status --porcelain)" ]; then
  fail "working tree must be clean before a release"
fi

original_branch="$(git symbolic-ref --quiet --short HEAD || true)"
original_commit="$(git rev-parse --verify HEAD)"

step "loading Apple release credentials"
readonly credentials_file="${MULTICA_RELEASE_ENV:-$HOME/.multica/release.env}"
[ -f "$credentials_file" ] ||
  fail "credential file not found: $credentials_file"

credentials_mode="$(file_mode "$credentials_file")"
readonly credentials_mode
case "$credentials_mode" in
  [0-7]00) ;;
  *)
    fail "credential file must not be accessible by group or others (run: chmod 600 $credentials_file)"
    ;;
esac

set -a
# shellcheck disable=SC1090
source "$credentials_file"
set +a

for variable_name in APPLE_ID APPLE_APP_SPECIFIC_PASSWORD APPLE_TEAM_ID; do
  if [ -z "${!variable_name:-}" ]; then
    fail "$variable_name is missing from $credentials_file"
  fi
done

step "checking GitHub authentication"
gh auth status --active --hostname github.com
GH_TOKEN="$(gh auth token --hostname github.com)"
[ -n "$GH_TOKEN" ] || fail "gh auth token returned an empty token"
export GH_TOKEN

step "checking the Developer ID signing identity"
signing_identities="$(security find-identity -v -p codesigning)"
readonly signing_identities
developer_identity="$(
  printf '%s\n' "$signing_identities" |
    grep -F 'Developer ID Application' |
    grep -F "($APPLE_TEAM_ID)" || true
)"
readonly developer_identity
[ -n "$developer_identity" ] ||
  fail "no Developer ID Application certificate found for team $APPLE_TEAM_ID"

if [ -n "${APPLE_SIGNING_IDENTITY:-}" ]; then
  printf '%s\n' "$developer_identity" | grep -Fq -- "$APPLE_SIGNING_IDENTITY" ||
    fail "APPLE_SIGNING_IDENTITY does not match the installed certificate"
  export CSC_NAME="$APPLE_SIGNING_IDENTITY"
fi

step "checking tag and GitHub Release readiness"
git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null ||
  fail "tag does not exist on origin: $TAG"
git fetch --quiet origin "refs/tags/$TAG:refs/tags/$TAG"
tag_commit="$(git rev-parse --verify "$TAG^{commit}")"
readonly tag_commit

is_draft="$(
  gh release view "$TAG" --repo "$REPOSITORY" --json isDraft --jq '.isDraft'
)"
readonly is_draft
[ "$is_draft" = "false" ] || fail "GitHub Release $TAG is still a draft"

release_assets="$(
  gh release view "$TAG" --repo "$REPOSITORY" --json assets --jq '.assets[].name'
)"
require_asset "$release_assets" "multica_darwin_amd64.tar.gz"
require_asset "$release_assets" "multica_darwin_arm64.tar.gz"

existing_mac_assets="$(
  printf '%s\n' "$release_assets" |
    grep -E '^(multica-desktop-.*-mac-(arm64|x64)\.(dmg|zip)(\.blockmap)?|latest(-x64)?-mac\.yml)$' || true
)"
if [ -n "$existing_mac_assets" ]; then
  printf '[release-desktop] Existing macOS Desktop assets for %s:\n%s\n' \
    "$TAG" "$existing_mac_assets" >&2
  fail "refusing to overwrite a partial release; inspect it and delete those assets explicitly before retrying"
fi

step "checking out $TAG"
checkout_changed=1
git switch --quiet --detach "$tag_commit"
derived_tag="$(git describe --tags --match 'v[0-9]*' --always --dirty)"
readonly derived_tag
[ "$derived_tag" = "$TAG" ] ||
  fail "Desktop packaging would derive version $derived_tag instead of $TAG"

step "installing locked dependencies"
pnpm install --frozen-lockfile

step "building, signing, notarizing, and uploading macOS Desktop artifacts"
pnpm --filter @multica/desktop package -- \
  --mac --arm64 --x64 --publish always

step "verifying published macOS Desktop assets"
release_assets="$(
  gh release view "$TAG" --repo "$REPOSITORY" --json assets --jq '.assets[].name'
)"

expected_mac_assets="$(printf '%s\n' \
  "multica-desktop-$VERSION-mac-arm64.dmg" \
  "multica-desktop-$VERSION-mac-arm64.dmg.blockmap" \
  "multica-desktop-$VERSION-mac-arm64.zip" \
  "multica-desktop-$VERSION-mac-arm64.zip.blockmap" \
  "latest-mac.yml" \
  "multica-desktop-$VERSION-mac-x64.dmg" \
  "multica-desktop-$VERSION-mac-x64.dmg.blockmap" \
  "multica-desktop-$VERSION-mac-x64.zip" \
  "multica-desktop-$VERSION-mac-x64.zip.blockmap" \
  "latest-x64-mac.yml")"

while IFS= read -r expected_asset; do
  require_asset "$release_assets" "$expected_asset"
done <<<"$expected_mac_assets"

published_mac_assets="$(
  printf '%s\n' "$release_assets" |
    grep -E '^(multica-desktop-.*-mac-(arm64|x64)\.(dmg|zip)(\.blockmap)?|latest(-x64)?-mac\.yml)$' || true
)"
published_mac_asset_count="$(
  printf '%s\n' "$published_mac_assets" | sed '/^$/d' | wc -l | tr -d ' '
)"
readonly published_mac_asset_count
[ "$published_mac_asset_count" -eq 10 ] ||
  fail "expected exactly 10 macOS Desktop assets, found $published_mac_asset_count"

release_url="$(
  gh release view "$TAG" --repo "$REPOSITORY" --json url --jq '.url'
)"
readonly release_url
log "Verified 10 macOS Desktop assets: $release_url"
