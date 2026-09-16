import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import type { IssueWakeup } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { WakeupsSection } from "./wakeups-section";
const mutate = vi.fn();
let wakeup: IssueWakeup;
let status = "queued";
vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws" }),
}));
vi.mock("@multica/core/issues", () => ({
  issueWakeupsOptions: () => ({ queryKey: ["wakeups"] }),
  issueTasksOptions: () => ({ queryKey: ["tasks"] }),
  useDisableIssueWakeup: () => ({ mutate, isPending: false }),
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
  status = "queued";
  wakeup = {
    id: "wake",
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
  it("exposes turn off and reveals the full prompt only on opening details", async () => {
    renderWithI18n(<WakeupsSection issueId="issue" />);
    expect(screen.queryByText("Check CI")).not.toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: /Turn off wakeup/ }));
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
    fireEvent.click(screen.getByRole("button", { name: /Turn off wakeup/ }));
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
  it("folds ended configurations into history and never offers a misleading switch", () => {
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
