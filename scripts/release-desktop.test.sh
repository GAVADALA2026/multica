#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly RELEASE_SCRIPT="$ROOT_DIR/scripts/release-desktop.sh"
readonly REAL_GIT="$(command -v git)"
readonly TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

fail() {
  echo "$1" >&2
  exit 1
}

assert_contains() {
  local output=$1 expected=$2
  grep -Fq -- "$expected" <<<"$output" ||
    fail "expected output to contain '$expected', got:\n$output"
}

assert_branch_restored() {
  local worktree=$1
  local branch
  branch="$($REAL_GIT -C "$worktree" branch --show-current)"
  [ "$branch" = main ] || fail "expected main to be restored, got '$branch'"
}

write_fake_tools() {
  local fixture=$1
  local fake_bin="$fixture/fake-bin"
  mkdir -p "$fake_bin"

  cat >"$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = remote ] && [ "${2:-}" = get-url ] && [ "${3:-}" = origin ]; then
  echo git@github.com:multica-ai/multica.git
  exit 0
fi
exec "$REAL_GIT" "$@"
EOF

  cat >"$fake_bin/uname" <<'EOF'
#!/usr/bin/env bash
echo Darwin
EOF

  cat >"$fake_bin/security" <<'EOF'
#!/usr/bin/env bash
echo '  1) ABCDEF "Developer ID Application: Multica Test (TESTTEAM123)"'
echo '     1 valid identities found'
EOF

  cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "auth status")
    # More than six lines catches a regression to `gh auth status | head -6`
    # under pipefail: gh receives SIGPIPE and the release aborts.
    for line in 1 2 3 4 5 6 7 8; do echo "auth status line $line"; done
    ;;
  "auth token")
    echo fake-token
    ;;
  "release view")
    case "$*" in
      *"--json isDraft"*) echo false ;;
      *"--json url"*) echo https://github.com/multica-ai/multica/releases/tag/v1.2.3 ;;
      *"--json assets"*)
        cat "$FAKE_STATE_DIR/initial-assets"
        if [ -f "$FAKE_STATE_DIR/published" ]; then
          cat "$FAKE_STATE_DIR/published-assets"
        fi
        ;;
      *) echo "unexpected gh invocation: $*" >&2; exit 90 ;;
    esac
    ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 91 ;;
esac
EOF

  cat >"$fake_bin/pnpm" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$FAKE_STATE_DIR/pnpm-invocations"
if [ "${1:-}" = install ]; then
  exit 0
fi
if [ -n "${FAKE_PACKAGE_EXIT:-}" ]; then
  exit "$FAKE_PACKAGE_EXIT"
fi
touch "$FAKE_STATE_DIR/published"
EOF

  for command_name in go node; do
    cat >"$fake_bin/$command_name" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  done
  chmod +x "$fake_bin"/*
}

new_fixture() {
  local name=$1
  local fixture="$TEST_ROOT/$name"
  local worktree="$fixture/worktree"
  local remote="$fixture/origin.git"

  mkdir -p "$worktree/scripts" "$fixture/state"
  cp "$RELEASE_SCRIPT" "$worktree/scripts/release-desktop.sh"
  chmod +x "$worktree/scripts/release-desktop.sh"
  printf '{"name":"release-test","version":"0.2.0"}\n' >"$worktree/package.json"

  "$REAL_GIT" -C "$worktree" init -q --initial-branch=main
  "$REAL_GIT" -C "$worktree" config user.name "Release Test"
  "$REAL_GIT" -C "$worktree" config user.email "release-test@example.com"
  "$REAL_GIT" -C "$worktree" add .
  "$REAL_GIT" -C "$worktree" commit -qm "tagged release"
  "$REAL_GIT" -C "$worktree" tag v1.2.3
  printf 'operator branch\n' >"$worktree/operator-marker"
  "$REAL_GIT" -C "$worktree" add operator-marker
  "$REAL_GIT" -C "$worktree" commit -qm "operator work"
  "$REAL_GIT" clone -q --bare "$worktree" "$remote"
  "$REAL_GIT" -C "$worktree" remote add origin "$remote"

  printf '%s\n' \
    multica_darwin_amd64.tar.gz \
    multica_darwin_arm64.tar.gz \
    >"$fixture/state/initial-assets"
  printf '%s\n' \
    multica-desktop-1.2.3-mac-arm64.dmg \
    multica-desktop-1.2.3-mac-arm64.dmg.blockmap \
    multica-desktop-1.2.3-mac-arm64.zip \
    multica-desktop-1.2.3-mac-arm64.zip.blockmap \
    latest-mac.yml \
    multica-desktop-1.2.3-mac-x64.dmg \
    multica-desktop-1.2.3-mac-x64.dmg.blockmap \
    multica-desktop-1.2.3-mac-x64.zip \
    multica-desktop-1.2.3-mac-x64.zip.blockmap \
    latest-x64-mac.yml \
    >"$fixture/state/published-assets"

  cat >"$fixture/release.env" <<'EOF'
APPLE_ID=release-test@example.com
APPLE_APP_SPECIFIC_PASSWORD=test-password
APPLE_TEAM_ID=TESTTEAM123
APPLE_SIGNING_IDENTITY='Multica Test'
EOF
  chmod 600 "$fixture/release.env"
  write_fake_tools "$fixture"
  printf '%s\n' "$fixture"
}

run_release() {
  local fixture=$1
  shift
  env \
    PATH="$fixture/fake-bin:$PATH" \
    REAL_GIT="$REAL_GIT" \
    FAKE_STATE_DIR="$fixture/state" \
    MULTICA_RELEASE_ENV="$fixture/release.env" \
    "$@" \
    bash "$fixture/worktree/scripts/release-desktop.sh" v1.2.3 2>&1
}

success_fixture="$(new_fixture success)"
success_output="$(run_release "$success_fixture")" ||
  fail "expected release to succeed, got:\n$success_output"
assert_contains "$success_output" "Verified 10 macOS Desktop assets"
assert_contains "$success_output" "Completed successfully"
assert_branch_restored "$success_fixture/worktree"
grep -Fxq -- 'install --frozen-lockfile' "$success_fixture/state/pnpm-invocations" ||
  fail "release did not install locked dependencies"
grep -Fxq -- '--filter @multica/desktop package -- --mac --arm64 --x64 --publish always' \
  "$success_fixture/state/pnpm-invocations" ||
  fail "release did not invoke the expected Desktop package command"

existing_fixture="$(new_fixture existing-assets)"
echo 'multica-desktop-1.2.3-mac-arm64.dmg' >>"$existing_fixture/state/initial-assets"
set +e
existing_output="$(run_release "$existing_fixture")"
existing_status=$?
set -e
[ "$existing_status" -ne 0 ] || fail "expected existing assets to stop the release"
assert_contains "$existing_output" "refusing to overwrite a partial release"
[ ! -f "$existing_fixture/state/pnpm-invocations" ] ||
  fail "release invoked pnpm after detecting existing assets"
assert_branch_restored "$existing_fixture/worktree"

failure_fixture="$(new_fixture package-failure)"
set +e
failure_output="$(run_release "$failure_fixture" FAKE_PACKAGE_EXIT=17)"
failure_status=$?
set -e
[ "$failure_status" -eq 17 ] ||
  fail "expected package exit 17 to be preserved, got $failure_status"
assert_contains "$failure_output" 'FAILED during "building, signing, notarizing, and uploading macOS Desktop artifacts" (exit 17)'
assert_branch_restored "$failure_fixture/worktree"

partial_fixture="$(new_fixture partial-upload)"
grep -Fvx -- latest-x64-mac.yml "$partial_fixture/state/published-assets" \
  >"$partial_fixture/state/published-assets.partial"
mv "$partial_fixture/state/published-assets.partial" "$partial_fixture/state/published-assets"
set +e
partial_output="$(run_release "$partial_fixture")"
partial_status=$?
set -e
[ "$partial_status" -ne 0 ] || fail "expected partial upload verification to fail"
assert_contains "$partial_output" "GitHub Release is missing required asset: latest-x64-mac.yml"
assert_branch_restored "$partial_fixture/worktree"

echo "✓ release-desktop validates preconditions, publishes deterministically, and restores the checkout"
