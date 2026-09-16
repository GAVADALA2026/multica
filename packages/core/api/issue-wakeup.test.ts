// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient } from "./client";
import { AgentTaskSchema } from "./schemas";
afterEach(() => vi.unstubAllGlobals());
const client = new ApiClient("https://api.example.test");
it("does not present malformed wakeup state as an empty list", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify([{ id: "wake", enabled: "false" }])),
      ),
  );
  await expect(client.listIssueWakeups("issue")).rejects.toThrow(
    "Could not load wakeups",
  );
});
it("preserves an empty wakeup list", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("[]")));
  await expect(client.listIssueWakeups("issue")).resolves.toEqual([]);
});
it("does not swallow a disable permission refusal", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response('{"error":"forbidden"}', { status: 403 }),
      ),
  );
  await expect(client.disableIssueWakeup("issue", "wake")).rejects.toThrow();
});

it("rejects malformed wakeup summary counts", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify([{ active_count: "2" }]))),
  );
  await expect(client.listIssueWakeupSummaries()).rejects.toThrow();
});
it("preserves empty summaries", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("[]")));
  await expect(client.listIssueWakeupSummaries()).resolves.toEqual([]);
});

it("preserves wakeup origin while accepting old task responses", () => {
  expect(
    AgentTaskSchema.parse({ id: "run", wakeup_id: "wake", status: "deferred" }),
  ).toMatchObject({ wakeup_id: "wake", status: "deferred" });
  expect(AgentTaskSchema.parse({ id: "run" }).wakeup_id).toBeUndefined();
});
