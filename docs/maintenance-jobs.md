# Operator-driven maintenance jobs

Maintenance runs through a separate HTTP listener in the API container. There
is no startup backfill, scheduler, background executor, or automatic restart.
The initial registered processor is `issue_status_category`, version 1. The
shared `maintenance_job` table also supports later database repair/backfill
processors with their own parameter, checkpoint and result schemas.

## Enable the local entrypoint

Set `MAINTENANCE_PORT` to an unused port (for example 6061) when deploying the
API. Unset means disabled. The bind address is always **127.0.0.1** and cannot
be overridden. Invalid configuration or a bind failure disables only this
listener and logs an error; check the listener before attempting maintenance.

Do not publish this port, add it to a Service/Ingress, or proxy these routes.
There is deliberately no application authentication. Access relies on container
exec/operational permissions and network-namespace isolation; another process
in the same namespace can also connect. Host-networked containers share the
host loopback namespace. This API is never mounted on the public router.
Avoid placing credentials or user content in job parameters or logs.

The following requests are run **inside the target API container**, using its
configured port. Merely enabling the listener does not create or advance a job.
No production execution is part of submitting this PR.

## Category backfill prerequisites

1. Deploy PR1's matching migration runner through 478. Inspect the actual CHECKs
   and functions, not just the migration ledger. Never execute historical 469
   directly. The API checks the ledger, compatibility vocabulary, mapping
   function, terminal spellings and its maintenance indexes.
2. Finish the full compatible backend rollout, including every writer. Capture
   deployment revision/replica evidence: the database cannot prove this.
   `parameters.writers_upgraded=true` is the operator's explicit attestation,
   not automatic proof of deployment.
3. Apply migrations 479–482. They only create the maintenance table and three
   unique indexes; indexes run concurrently in separate migration files with
   invalid-index cleanup hooks. No issue-status rows are changed.
4. Record baseline API latency/error rate, row-lock waits, DB I/O, WAL rate and
   replica lag. Choose batch/delay limits against the environment's existing
   alert budgets, beginning with a bounded canary. Pause if any metric exceeds
   its agreed budget; reduce batch size/increase delay before resuming.
   No fixed throughput or historical row count is a current safety guarantee.

## Dry-run, then apply

Create a dry-run record (dry-run is also the default when omitted):

```bash
curl --fail-with-body --noproxy '*' http://127.0.0.1:6061/maintenance/jobs \
  -H 'Content-Type: application/json' \
  -d '{"job_type":"issue_status_category","job_version":1,"scope_key":"database","idempotency_key":"status-category-v1-check-001","dry_run":true,"options":{"batch_size":500,"delay_ms":100,"lock_timeout_ms":500,"statement_timeout_ms":3000},"parameters":{}}'
```

Keep the returned `job.id`. Dry-run persists its own progress/report but never
changes category data. Each advance scans at most the configured page size.
Run one canary batch, inspect the response and metrics, then continue:

```bash
python3 scripts/maintenance-job.py --port 6061 --job JOB_UUID --max-batches 1
python3 scripts/maintenance-job.py --port 6061 --job JOB_UUID --max-batches 100 --max-seconds 300
```

The helper is optional and uses Python's standard library. If Python or the
checkout is absent in the image, invoke the same endpoints with curl from the
container's admin shell. A batch invocation is:

```bash
curl --fail-with-body --noproxy '*' http://127.0.0.1:6061/maintenance/jobs/JOB_UUID/advance \
  -H 'Content-Type: application/json' -d '{"revision":0}'
```

Use the current returned revision for the next intentional batch. Retry an
ambiguous request with its **original** revision. A stale revision returns 409
with current state and performs no work. GET
`/maintenance/jobs/JOB_UUID` reads the primary DB and is always safe to retry.
429 means the persisted `next_allowed_at` has not elapsed. 409 without a job
means another request currently owns the row lock; back off rather than queue.

Once dry-run is completed, create a new record using a new idempotency key,
`"dry_run":false`, and `"parameters":{"writers_upgraded":true}`. Repeat the
canary and bounded advances. The initial options are conservative starting
values for testing, not environment-specific production guarantees.

Creating with the same idempotency key and identical normalized parameters
returns the original job. Reusing that key with different input conflicts.
Only one ready/paused record per job type and scope is permitted, including
dry-runs. Finish or explicitly cancel the existing record before starting
another. Different logical scopes must be disjoint; new processors that allow
overlapping scopes must implement an additional conflict guard.

## Pause, tune, resume, failures

Stopping calls stops progress. POST `/pause`, `/resume`, or `/cancel` under
the job URL with `{"revision":CURRENT_REVISION}` changes durable state across
all replicas. These operations may return busy while a short batch holds the
job row; retry after it finishes. Cancellation preserves the already committed
data and audit record; it does not reverse a backfill.

While paused, POST `/configure` with:

```json
{"revision":3,"options":{"batch_size":100,"delay_ms":1000,"lock_timeout_ms":250,"statement_timeout_ms":2000}}
```

This replaces operational limits, preserving checkpoint and immutable task
parameters. Resume with the new revision. Limits: batch 1–5000, delay 1–60000ms,
and 1 <= lock timeout <= statement timeout <= 5000ms. Omitted/zero limits use
defaults. Increasing/decreasing limits cannot erase an existing delay.

Each batch locks the maintenance row NOWAIT, scans an indexed ID page, updates
only still-legacy categories from their current values, and commits data,
checkpoint, counters and revision in one transaction. It includes system,
custom and archived rows, preserving all fields except category. No issue
update events or agent runs are emitted. It does not use SKIP LOCKED or OFFSET.

On SQL failure the batch savepoint rolls back all data changes. The outer
transaction records a paused state and error without advancing the checkpoint.
Timeout/deadlock/serialization errors are marked retryable; the helper retries
with capped exponential backoff (default 3, configurable up to 10), using the
new revision to resume that failed batch. Other errors stop the helper.
A previously paused job is not automatically resumed without `--resume`.
There is no automatic retry after process exit. The helper has batch and wall
time budgets; Ctrl-C stops driving requests.

If the connection/container disappears before commit, data and progress both
roll back. If it disappears after commit, another replica can resume from the
stored cursor. If cancellation prevents even the error record from committing,
the checkpoint stays unchanged and server logs/client transport error provide
the failure evidence; inspect GET before retrying. Graceful shutdown drains
the internal listener before closing the database pool.

## Completion and evidence

Application runs move from the scan phase into a fresh, independently paged
verification phase. Verification rejects residual legacy values, unknown
categories and noncanonical system pairs. A bad page pauses with the first
offending status ID. Fix the cause, then resume; for residuals behind the scan
cursor, cancel and start a new pass after eliminating old writers.

Only a complete verification pass produces `status=completed` and
`result.remaining_legacy=0`. That report depends on the no-old-writers rollout
precondition; a paged scan cannot certify that an old writer will never write
again. Dry-run completion only means the read-only survey completed, not that
the data has been converted. Dry-run counts and `would_update` are observations
during a paged scan, not a globally consistent snapshot. Progress is cumulative;
`updated` counts actual committed updates, while `would_update` counts old
values observed before any concurrent application normalization. Neither is a
live remaining-row count. Use a fresh dry-run to refresh backlog evidence.

Save the job JSON, deployment evidence, monitoring/canary results and operator
session log. Completed/cancelled rows are retained; repeated maintenance creates
new records rather than resetting history. Down migrations intentionally retain
state and indexes. Keep the compatible backend and fix forward on failures.

PR3 remains separate: no strict CHECK, validation DDL, legacy-reader removal,
or wire-protocol retirement is triggered by this job. Installed-client API
compatibility remains after backfill completion.

## Adding a processor

Register a code-defined `maintenance.Processor` with a type and version. Validate
scope and parameters, implement preflight and one bounded Step, and return
versioned checkpoint/progress/result JSON objects plus a completion flag.
A processor uses only the supplied primary-database transaction. Unknown
types/versions and malformed state fail closed. This is not an arbitrary SQL
execution endpoint. Checkpoint atomicity does not cover external side effects;
such processors need their own idempotency/recovery design before registration.
