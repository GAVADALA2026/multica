import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import type { IssueWakeup } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { WakeupsSection } from "./wakeups-section";
const mutate = vi.fn();
const enable = vi.fn();
let pending = false;
let wakeup: IssueWakeup;
let status = "queued";
vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws" }),
}));
vi.mock("@multica/core/issues", () => ({
  issueWakeupsOptions: () => ({ queryKey: ["wakeups"] }),
  issueTasksOptions: () => ({ queryKey: ["tasks"] }),
  useDisableIssueWakeup: () => ({ mutate, isPending: false }),
  useEnableIssueWakeup: () => ({ mutateAsync: enable, isPending: pending }),
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) => ({
    data: queryKey[0] === "wakeups" ? [wakeup] : [{ id: "task", status }],
  }),
}));
vi.mock("../../common/task-transcript", () => ({
  TranscriptButton: () => <button>Transcript</button>,
}));
beforeEach(() => {
  mutate.mockReset();
  enable.mockReset().mockResolvedValue(undefined);
  pending = false;
  status = "queued";
  wakeup = {
    id: "wake",
    revision: 2,
    issue_id: "issue",
    agent_id: "agent",
    agent_name: "Emacs",
    instruction: "Check CI",
    kind: "every",
    mode: "continuous",
    event_types: [],
    interval_seconds: 3600,
    enabled: true,
    disabled_at: null,
    last_task_id: "task",
    last_error: null,
    timezone: "UTC",
    next_fire_at: null,
    cron_expression: null,
    filter_agent_id: null,
    filter_task_id: null,
  };
});
describe("Wakeups sidebar", () => {
  it("exposes the toggle and reveals the full prompt only on opening details", async () => {
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByText("Check CI")).not.toBeVisible();
    fireEvent.click(screen.getByRole("switch", { name: "Wakeup for Emacs" }));
    expect(mutate).toHaveBeenCalledWith("wake", expect.any(Object));
    fireEvent.click(screen.getByRole("button", { name: /Wake Emacs/ }));
    await waitFor(() => expect(screen.getByText("Check CI")).toBeVisible());
    expect(screen.getByRole("button", { name: "Transcript" })).toBeVisible();
  });
  it("keeps a consumed one-shot queued run in the current list with withdrawal", () => {
    wakeup.enabled = false;
    wakeup.kind = "event";
    wakeup.mode = "once";
    wakeup.event_types = ["task.completed"];
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByText(/Ended/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Cancel pending run" }));
    expect(mutate).toHaveBeenCalled();
  });
  it("keeps an already claimed single run visible without offering withdrawal", () => {
    wakeup.enabled = false;
    status = "running";
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByText(/Ended/)).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Turn off wakeup/ }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: /Wake Emacs/ })).toBeVisible();
  });
  it("folds ended configurations into history with an off toggle", () => {
    wakeup.enabled = false;
    status = "completed";
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByRole("button", { name: /Wake Emacs/ })).toBeNull();
    expect(screen.queryByRole("switch")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ended 1" }));
    expect(screen.getByRole("button", { name: /Wake Emacs/ })).toBeVisible();
  });
  it("uses server run status while the run detail query is still incomplete", () => {
    wakeup.enabled = false;
    wakeup.last_task_id = "missing";
    wakeup.last_task_status = "dispatched";
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByText(/Ended/)).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Turn off wakeup/ }),
    ).toBeNull();
  });
});

it("re-enables a manually disabled recurring configuration with its revision", async () => {
  wakeup.enabled = false;
  wakeup.disabled_at = "2026-09-16T00:00:00Z";
  status = "completed";
  renderWithI18n(<WakeupsSection issueId="issue" />);
  fireEvent.click(screen.getByRole("button", { name: "Ended 1" }));
  const toggle = screen.getByRole("switch", { name: "Wakeup for Emacs" });
  expect(toggle).not.toBeChecked();
  fireEvent.click(toggle);
  await waitFor(() =>
    expect(enable).toHaveBeenCalledWith({ id: "wake", revision: 2 }),
  );
  expect(toggle).not.toBeChecked(); // Wait for server state; no optimistic success.
});
it("blocks re-enabling on terminal issues", () => {
  wakeup.enabled = false;
  status = "completed";
  renderWithI18n(<WakeupsSection issueId="issue" closed />);
  fireEvent.click(screen.getByRole("button", { name: "Ended 1" }));
  expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
  fireEvent.click(screen.getByRole("switch"));
  expect(enable).not.toHaveBeenCalled();
  expect(mutate).not.toHaveBeenCalled();
  expect(screen.getByText(/cannot be enabled/)).toBeVisible();
});
it("requires a new future time for an expired one-shot", async () => {
  wakeup.enabled = false;
  wakeup.disabled_at = "2020-01-01T00:00:00Z";
  wakeup.kind = "at";
  wakeup.mode = "once";
  wakeup.next_fire_at = "2020-01-01T00:00:00Z";
  status = "completed";
  renderWithI18n(<WakeupsSection issueId="issue" />);
  fireEvent.click(screen.getByRole("button", { name: "Ended 1" }));
  expect(screen.queryByRole("switch")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Reschedule" }));
  const input = await screen.findByLabelText(/Time \(/);
  fireEvent.change(input, { target: { value: "2020-01-01T12:00" } });
  fireEvent.submit(input.closest("form")!);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Choose a future time",
  );
  expect(enable).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: "2099-01-01T12:00" } });
  fireEvent.submit(input.closest("form")!);
  await waitFor(() =>
    expect(enable).toHaveBeenCalledWith({
      id: "wake",
      revision: 2,
      at: new Date("2099-01-01T12:00").toISOString(),
      rearm: true,
    }),
  );
});
it("uses explicit resubscribe for a completed one-shot event", async () => {
  wakeup.enabled = false;
  wakeup.kind = "event";
  wakeup.mode = "once";
  status = "completed";
  renderWithI18n(<WakeupsSection issueId="issue" />);
  fireEvent.click(screen.getByRole("button", { name: "Ended 1" }));
  expect(screen.queryByRole("switch")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Resubscribe" }));
  await waitFor(() =>
    expect(enable).toHaveBeenCalledWith({
      id: "wake",
      revision: 2,
      rearm: true,
    }),
  );
});
it("disables controls during a pending mutation", () => {
  pending = true;
  renderWithI18n(<WakeupsSection issueId="issue" />);
  expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
  fireEvent.click(screen.getByRole("switch"));
  expect(enable).not.toHaveBeenCalled();
  expect(mutate).not.toHaveBeenCalled();
});
