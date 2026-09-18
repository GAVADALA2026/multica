import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AutopilotTrigger } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";

// The editor a trigger row opens (MUL-7478). Before it, an existing schedule
// could only be deleted and recreated: the autopilot dialog's panel speaks for
// one schedule, and the detail page listed triggers read-only.

const mockUpdateTrigger = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-test" }));

vi.mock("@multica/core/autopilots/queries", () => ({
  cronPreviewOptions: (wsId: string, expr: string, tz: string) => ({
    queryKey: ["cron-preview", wsId, expr, tz],
    queryFn: async () => ({ next_runs: ["2126-07-14T01:00:00Z"] }),
    retry: false,
  }),
}));

vi.mock("@multica/core/autopilots/mutations", () => ({
  useUpdateAutopilotTrigger: () => ({ mutateAsync: mockUpdateTrigger }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("./pickers/timezone-picker", () => ({
  TimezonePicker: ({ value }: { value: string }) => <div data-testid="timezone-picker">{value}</div>,
}));

import { EditScheduleTriggerDialog } from "./edit-schedule-trigger-dialog";

const AUTOPILOT_ID = "ap-1";

function trigger(overrides: Partial<AutopilotTrigger> = {}): AutopilotTrigger {
  return {
    id: "trg-evening",
    autopilot_id: AUTOPILOT_ID,
    kind: "schedule",
    enabled: true,
    cron_expression: "TZ=Asia/Bangkok 0 */3 * * *",
    timezone: "Asia/Bangkok",
    next_run_at: null,
    webhook_token: null,
    label: null,
    last_fired_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderDialog(trig: AutopilotTrigger = trigger()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onOpenChange = vi.fn();
  const result = renderWithI18n(
    <QueryClientProvider client={qc}>
      <EditScheduleTriggerDialog
        open
        onOpenChange={onOpenChange}
        autopilotId={AUTOPILOT_ID}
        trigger={trig}
      />
    </QueryClientProvider>,
  );
  return { ...result, onOpenChange };
}

const saveButton = () => screen.getByRole("button", { name: "Save" });

describe("EditScheduleTriggerDialog", () => {
  beforeEach(() => {
    mockUpdateTrigger.mockReset().mockResolvedValue({ id: "trg-evening" });
  });

  it("opens on the schedule the row already runs, not on a default", () => {
    renderDialog();

    // The stored zone and interval, read back from the row — seeding the editor
    // with its own 09:00 default would be a proposal dressed as the trigger's
    // state, which is how MUL-5649 lost a save under a success toast.
    expect(screen.getByTestId("timezone-picker")).toHaveTextContent("Asia/Bangkok");
    expect(screen.getByRole("button", { name: "At an interval", pressed: true })).toBeInTheDocument();
    expect(screen.getByDisplayValue("3")).toBeInTheDocument();
  });

  it("patches this trigger alone, carrying the zone with the expression", async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderDialog();

    await user.click(screen.getByRole("button", { name: "At a time" }));
    await user.click(saveButton());

    await waitFor(() => expect(mockUpdateTrigger).toHaveBeenCalledTimes(1));
    const patch = mockUpdateTrigger.mock.calls[0]?.[0];
    expect(patch).toMatchObject({
      autopilotId: AUTOPILOT_ID,
      triggerId: "trg-evening",
      timezone: "Asia/Bangkok",
      enabled: true,
    });
    expect(patch.cron_expression).toContain("Asia/Bangkok");
    // An untouched label is left out of the PATCH: sending this dialog's
    // reading of it back would turn an absent label into an empty one.
    expect(patch.label).toBeUndefined();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("sends the label once the user gives the schedule one", async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.type(screen.getByPlaceholderText("e.g. Weekday morning"), "Evening sweep");
    await user.click(saveButton());

    await waitFor(() => expect(mockUpdateTrigger).toHaveBeenCalledTimes(1));
    expect(mockUpdateTrigger.mock.calls[0]?.[0].label).toBe("Evening sweep");
  });

  it("pauses the schedule without deleting it", async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.click(screen.getByRole("switch", { name: "Enabled" }));
    await user.click(saveButton());

    await waitFor(() => expect(mockUpdateTrigger).toHaveBeenCalledTimes(1));
    const patch = mockUpdateTrigger.mock.calls[0]?.[0];
    expect(patch.enabled).toBe(false);
    // The cron survives the pause — this is the disabled badge the detail page
    // already renders, not a delete.
    expect(patch.cron_expression).toContain("*/3");
  });

  it("keeps the dialog open when the write fails, with the server's reason", async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderDialog();
    mockUpdateTrigger.mockRejectedValueOnce(new Error("cron_expression is invalid"));

    await user.click(saveButton());

    await waitFor(() => expect(mockUpdateTrigger).toHaveBeenCalledTimes(1));
    const { toast } = await import("sonner");
    expect(toast.error).toHaveBeenCalledWith("cron_expression is invalid");
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
