#!/usr/bin/env python3
"""Drive bounded batches through the container-local maintenance API."""
import argparse
import datetime
import json
import sys
import time
import urllib.error
import urllib.request
import uuid


def request(base, path, body=None, timeout=20):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        base + path, data=data, headers={"Content-Type": "application/json"}
    )
    # Container loopback must never go through an operator's HTTP proxy.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        response = opener.open(req, timeout=timeout)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, json.load(response)


def parse_timestamp(value):
    # Python 3.9 fromisoformat only accepts certain fractional widths; Go's
    # timestamp encoder drops trailing zeros from PostgreSQL microseconds.
    pattern = "%Y-%m-%dT%H:%M:%S.%f%z" if "." in value else "%Y-%m-%dT%H:%M:%S%z"
    return datetime.datetime.strptime(value.replace("Z", "+00:00"), pattern)


def drive(base, job_id, max_batches, max_retries, max_seconds, resume=False):
    deadline = time.monotonic() + max_seconds
    path = "/maintenance/jobs/" + job_id

    def call(suffix="", body=None):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("run time budget exhausted; query the job before continuing")
        return request(base, path + suffix, body, min(20, remaining))

    code, payload = call()
    if code != 200:
        raise RuntimeError(payload)
    job = payload["job"]
    if resume and job["status"] == "paused":
        code, payload = call("/resume", {"revision": job["revision"]})
        if code != 200:
            raise RuntimeError(payload)
        job = payload["job"]
    retries = 0
    for _ in range(max_batches):
        print(json.dumps(job), flush=True)
        if job["status"] == "completed":
            return job
        if job["status"] != "ready":
            raise RuntimeError("job is " + job["status"] + "; inspect last_error before resuming")
        next_at = parse_timestamp(job["next_allowed_at"])
        delay = max(0, next_at.timestamp() - time.time())
        if delay >= deadline - time.monotonic():
            break
        time.sleep(delay)
        # Keep this revision across transport retries. A lost acknowledgement
        # must never turn into an unrequested additional batch.
        revision = job["revision"]
        while True:
            try:
                code, payload = call("/advance", {"revision": revision})
            except (urllib.error.URLError, TimeoutError):
                if retries >= max_retries or time.monotonic() >= deadline:
                    raise
                retries += 1
                time.sleep(min(0.25 * 2 ** (retries - 1), 5))
                continue
            if code == 200:
                job = payload["job"]
                retries = 0
                break
            if code == 409 and payload.get("job", {}).get("revision", revision) != revision:
                # Another replica committed, or our earlier response was lost.
                job = payload["job"]
                break
            if not payload.get("retryable") or retries >= max_retries:
                raise RuntimeError(payload)
            retries += 1
            time.sleep(min(0.25 * 2 ** (retries - 1), 5))
            if payload.get("job", {}).get("status") == "paused":
                # The server rolled back the batch and persisted its failure.
                job = payload["job"]
                code, payload = call("/resume", {"revision": job["revision"]})
                if code != 200:
                    raise RuntimeError(payload)
                job = payload["job"]
                revision = job["revision"]
        if time.monotonic() >= deadline:
            break
    print(json.dumps(job), flush=True)
    if job["status"] not in ("ready", "completed"):
        raise RuntimeError("job stopped in " + job["status"])
    return job


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", required=True, type=int)
    parser.add_argument("--job", required=True, type=uuid.UUID)
    parser.add_argument("--max-batches", type=int, default=1)
    parser.add_argument("--max-retries", type=int, default=3)
    parser.add_argument("--max-seconds", type=int, default=300)
    parser.add_argument("--resume", action="store_true", help="explicitly resume a paused job first")
    args = parser.parse_args()
    if not 1 <= args.port <= 65535 or args.max_batches < 1 or not 0 <= args.max_retries <= 10 or args.max_seconds < 1:
        parser.error("invalid port or run limits")
    drive("http://127.0.0.1:" + str(args.port), str(args.job),
          args.max_batches, args.max_retries, args.max_seconds, args.resume)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, urllib.error.URLError, TimeoutError, KeyboardInterrupt) as error:
        print(str(error) + "\nNo automatic restart; inspect the persisted job before resuming.", file=sys.stderr)
        sys.exit(1)
