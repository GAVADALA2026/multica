import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, matchesGlob } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { checkGate, decideScopes, filters } from "./ci-scope.mjs";

// The filters use only literal paths, * and **; Node's matcher exercises those
// against realistic changes, including files removed or renamed by a PR.
function filterFiles(files) {
  return Object.fromEntries(Object.entries(filters).map(([scope, patterns]) => [
    scope, String(files.some((file) => patterns.some((pattern) => matchesGlob(file, pattern)))),
  ]));
}

for (const [name, files, selected] of [
  ["readme only", ["README.md"], []],
  ["docs only", ["apps/docs/content/docs/guide.mdx"], ["quality"]],
  ["web changelog", ["apps/web/features/landing/i18n/en.ts"], ["frontend", "quality"]],
  ["mobile UI", ["apps/mobile/app/index.tsx"], ["quality"]],
  ["migration only", ["server/migrations/999_example.up.sql"], ["backend", "sqlc"]],
  ["agent process code", ["server/pkg/agent/cursor_background.go"], ["backend", "runtime"]],
  ["daemon dependency", ["server/internal/skill/service.go"], ["backend", "runtime"]],
  ["Go dependencies", ["server/go.mod", "server/go.sum"], ["backend", "runtime"]],
  ["Helm only", ["deploy/helm/multica/templates/deployment.yaml"], ["scripts"]],
  ["container entrypoint", ["docker/entrypoint.sh"], ["scripts"]],
  ["selfhost config", [".env.example"], ["scripts", "installer"]],
  ["shell installer", ["scripts/install.sh"], ["scripts", "installer"]],
  ["PowerShell installer", ["scripts/install.ps1.test.ps1"], ["installer"]],
  ["cleanup script", ["scripts/drop-database.sh"], ["scripts"]],
  ["performance harness", ["scripts/perf-compare.test.sh"], ["scripts"]],
  ["reserved slug source", ["server/internal/handler/reserved_slugs.json"], ["backend", "runtime", "scripts"]],
  ["reserved slug output", ["packages/core/paths/reserved-slugs.ts"], ["frontend", "quality", "scripts"]],
  ["cross-module runtime contract", ["packages/core/runtimes/cli-version.ts"], ["frontend", "backend", "runtime", "quality"]],
  ["lockfile", ["pnpm-lock.yaml"], ["frontend", "quality"]],
  ["package patch", ["patches/example.patch"], ["frontend", "quality"]],
  ["radius policy", ["scripts/check-ui-radius-tokens.mjs"], ["quality"]],
  ["new bitmap", ["apps/web/public/hero.png"], ["frontend", "quality", "images"]],
  ["mixed docs and migration", ["apps/docs/content/guide.mdx", "server/migrations/999_example.up.sql"], ["quality", "backend", "sqlc"]],
  ["CI configuration", [".github/ci-paths.json"], Object.keys(filters)],
]) {
  test(`PR and main select only affected scopes: ${name}`, () => {
    for (const event of ["pull_request", "push"]) {
      const outputs = decideScopes(event, filterFiles(files));
      assert.equal(outputs.full, "false");
      assert.deepEqual(Object.keys(filters).filter((scope) => outputs[scope] === "true").sort(),
        [...selected].sort());
    }
  });
}

test("scheduled and manual runs select every scope without a path-filter result", () => {
  for (const event of ["schedule", "workflow_dispatch"]) {
    assert.ok(Object.values(decideScopes(event, {})).every((value) => value === "true"));
  }
});

test("missing, malformed and unsupported filter results fail closed", () => {
  for (const value of [undefined, "", "unknown", true]) {
    assert.throws(() => decideScopes("push", { ...filterFiles([]), backend: value }), /backend/);
  }
  assert.throws(() => decideScopes("unknown", filterFiles([])), /Unsupported/);
});

const mapping = { tests: "backend", sqlc: "sqlc" };
function needs(backend = "true", sqlc = "false") {
  return {
    changes: { result: "success", outputs: { backend, sqlc } },
    tests: { result: backend === "true" ? "success" : "skipped" },
    sqlc: { result: sqlc === "true" ? "success" : "skipped" },
  };
}

test("gates accept success and only explicitly selected skips", () => {
  for (const backend of ["true", "false"]) {
    for (const sqlc of ["true", "false"]) checkGate(needs(backend, sqlc), mapping);
  }
});

test("failed or cancelled changes cannot turn skipped tests into a green gate", () => {
  for (const result of ["failure", "cancelled", "skipped", undefined]) {
    const input = needs("false", "false");
    input.changes.result = result;
    assert.throws(() => checkGate(input, mapping), /Path filtering/);
  }
});

test("an expected job cannot fail, disappear or be skipped", () => {
  for (const result of ["failure", "cancelled", "skipped", undefined]) {
    const input = needs();
    input.tests.result = result;
    assert.throws(() => checkGate(input, mapping), /tests/);
  }
  const input = needs();
  delete input.tests;
  assert.throws(() => checkGate(input, mapping), /missing/);
});

test("unselected jobs cannot hide failures, and missing outputs cannot authorize skips", () => {
  const input = needs("false");
  input.tests.result = "failure";
  assert.throws(() => checkGate(input, mapping), /tests/);
  for (const value of [undefined, "", "unknown"]) {
    const missing = needs("false");
    missing.changes.outputs.backend = value;
    assert.throws(() => checkGate(missing, mapping), /scope/);
  }
});

test("adding a dependency without checking it cannot silently pass", () => {
  const input = needs();
  input.extra = { result: "failure" };
  assert.throws(() => checkGate(input, mapping), /Unchecked dependency/);
});

test("the CLI writes real Actions outputs and exits nonzero on a failed gate", (t) => {
  const dir = mkdtempSync(join(tmpdir(), "ci-scope-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const output = join(dir, "output");
  const script = fileURLToPath(new URL("./ci-scope.mjs", import.meta.url));
  const run = spawnSync(process.execPath, [script, "decide"], {
    env: { ...process.env, EVENT_NAME: "push", FILTER_RESULTS: JSON.stringify(filterFiles(["README.md"])), GITHUB_OUTPUT: output },
    encoding: "utf8",
  });
  assert.equal(run.status, 0, run.stderr);
  assert.match(readFileSync(output, "utf8"), /^backend=false$/m);
  const failed = needs();
  failed.tests.result = "failure";
  const gate = spawnSync(process.execPath, [script, "gate"], {
    env: { ...process.env, NEEDS_JSON: JSON.stringify(failed), JOB_SCOPES: JSON.stringify(mapping) },
    encoding: "utf8",
  });
  assert.equal(gate.status, 1);
  assert.match(gate.stderr, /tests/);
});
