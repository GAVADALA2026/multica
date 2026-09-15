# CI scope and full validation

Pull requests and pushes to `main` select checks using
[`ci-paths.json`](ci-paths.json). JSON is also valid YAML, so the same file is
read by `dorny/paths-filter` and the local regression tests. On pushes, the base
is the event's `before` SHA; PRs use the action's pull-request file list.

The main CI workflow also runs all scopes daily at 03:23 UTC and through
**Actions → CI → Run workflow**. These runs include the repeated macOS
finalization stress tests. They have a separate concurrency group from pushes,
so a merge does not cancel the daily validation. Existing manual desktop/UI
performance workflows, weekly OpenClaw smoke and release verification retain
their separate triggers.

| Scope | Coverage |
| --- | --- |
| `frontend` | Web/desktop/shared-package build, typecheck, lint and disjoint test shards |
| `quality` | Repository UI reachability, radius checks including mobile, advisory knip |
| `backend` | Full Linux Go race suites, build, integration-test vet, migrations and vulnerability scan |
| `sqlc` | Generated query drift |
| `runtime` | macOS/Windows Cursor lifecycle and Windows runtime regressions |
| `scripts` | Self-host configuration, development scripts, Helm/entrypoint/build contracts and reserved-slug generation |
| `installer` | Installer fixtures on Linux, macOS and Windows |
| `images` | PR bitmap size check against the exact base SHA |

UI exports, radius and knip checks share the `frontend-quality` composite
action. When `frontend` is selected, `frontend-build` runs it using its existing
dependency install. Otherwise, `quality` changes select a standalone runner via
the derived `quality_only` output. Docs/mobile-only changes still get these
checks, while web/desktop and daily/manual full runs do not pay for a second
quality install. Knip remains advisory in both callers.

Runtime selection deliberately includes all internal/shared Go sources: the
daemon tests import CLI, handler and service code as well as the agent package.
This includes `server/pkg/db/generated`: query-generated Go changes can break
native test compilation even when the selected tests do not query a database.
Inspect the test dependency graph with `go list -deps -test` from `server/`
before narrowing the filter. Include `./internal/daemon`,
`./internal/daemon/execenv`, `./internal/daemon/repocache` and `./pkg/agent`.
The current scope is broader than process code; it also protects the platform
suites' transitive compilation dependencies.
Migration-only and Helm-only changes do not need native process tests. Linux
Cursor tests are already included in the full Linux race suites. macOS keeps
one finalization pass without race instrumentation plus its cgo-disabled
ownership checks on ordinary changes; five-run stress passes are full-run only.

The `frontend` and `backend` aggregate checks require `changes` to succeed and
every selected dependency to succeed. Only an explicit `false` path result can
authorize a skipped dependency. Failed filtering, missing outputs, cancelled or
missing expected jobs all fail the gate. Job-level filtering avoids allocating
the PostgreSQL/Redis services when backend tests are not selected.
The backend aggregate also owns the three-platform installer matrix: selected
installer failures cannot leave the aggregate green, and unrelated changes
may explicitly skip it.

When adding a check, include its sources, fixtures, generated inputs and
cross-package dependencies in the filter. Update scope regression scenarios
when changing a boundary. Keep repo-wide radius checking in the shared quality
action so a shared change does not run it twice through Mobile Verify.
Gate regression tests read the production `needs`, `JOB_SCOPES`, conditions and
outputs from the workflow. They exercise mixed selections and unsuccessful or
missing jobs, including standalone quality checks and the installer matrix.

Local validation (Node 22.13+):

```sh
node --test scripts/ci-scope.test.mjs scripts/check-image-budget.test.mjs
actionlint .github/workflows/ci.yml .github/workflows/mobile-verify.yml
git diff --check
```

The image regression uses a temporary local Git repository with a shallow head
and fetched base; it verifies oversize rejection, exemptions and shrinking
images without requiring the intervening history. No application services or
real agent CLIs are needed for these CI tests.
