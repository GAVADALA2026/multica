"use client";

import { useState } from "react";
import { useUpdateAutopilotTrigger } from "@multica/core/autopilots/mutations";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Switch } from "@multica/ui/components/ui/switch";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { toast } from "sonner";
import type { AutopilotTrigger } from "@multica/core/types";
import { ScheduleEditor } from "./schedule-editor/schedule-editor";
import { parseCron, toCron } from "./schedule-editor/cron-mapping";
import { useScheduleSubmitGate } from "./schedule-editor/validate";
import type { ScheduleConfig } from "./schedule-editor/model";
import { useT } from "../../i18n";

// The only place in the UI where an existing schedule can be changed. The
// autopilot dialog's panel speaks for the autopilot's one schedule; a trigger
// row speaks for itself, which is what an autopilot carrying several of them
// needs (MUL-7478). Mounted per open so the editor always hydrates from the
// row as it stands now — a stale snapshot here would write back a cron the
// user never saw.
export function EditScheduleTriggerDialog({
  open,
  onOpenChange,
  autopilotId,
  trigger,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  autopilotId: string;
  trigger: AutopilotTrigger;
}) {
  if (!open) return null;
  return (
    <EditScheduleTriggerDialogBody
      onOpenChange={onOpenChange}
      autopilotId={autopilotId}
      trigger={trigger}
    />
  );
}

function EditScheduleTriggerDialogBody({
  onOpenChange,
  autopilotId,
  trigger,
}: {
  onOpenChange: (open: boolean) => void;
  autopilotId: string;
  trigger: AutopilotTrigger;
}) {
  const { t } = useT("autopilots");
  const wsId = useWorkspaceId();
  const updateTrigger = useUpdateAutopilotTrigger();
  // `parseCron` round-trips anything the server stored: an expression outside
  // the structured model comes back as an advanced config holding the raw
  // fields, which the editor renders in its expression row. So every schedule
  // row is editable here, not only the ones the pickers can describe.
  const [config, setConfig] = useState<ScheduleConfig>(() =>
    parseCron(trigger.cron_expression ?? "", trigger.timezone ?? "UTC"),
  );
  const [label, setLabel] = useState(trigger.label ?? "");
  const [enabled, setEnabled] = useState(trigger.enabled);
  const [submitting, setSubmitting] = useState(false);
  const scheduleGate = useScheduleSubmitGate(wsId);
  const canSubmit = !submitting && scheduleGate.scheduleValid;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      if (!(await scheduleGate.ensureAccepted(config))) {
        setSubmitting(false);
        return;
      }
      const cronExpr = toCron(config);
      if (!cronExpr.trim()) {
        setSubmitting(false);
        return;
      }
      // Only what this dialog owns, and of that only what moved: the PATCH
      // preserves any field it is not sent, so an untouched label is left
      // alone instead of being rewritten as this dialog's reading of it —
      // which would turn an absent label into an empty one.
      const trimmedLabel = label.trim();
      await updateTrigger.mutateAsync({
        autopilotId,
        triggerId: trigger.id,
        cron_expression: cronExpr,
        timezone: config.timezone || undefined,
        enabled,
        label: trimmedLabel === (trigger.label ?? "") ? undefined : trimmedLabel,
      });
      toast.success(t(($) => $.edit_trigger_dialog.toast_updated));
      onOpenChange(false);
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.edit_trigger_dialog.toast_update_failed),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogTitle>{t(($) => $.edit_trigger_dialog.title)}</DialogTitle>
        {/* Same min-w-0 as the add dialog: the cron readback is one unbreakable
            line that would otherwise push the grid track past the dialog. */}
        <div className="min-w-0 space-y-4 pt-2">
          <ScheduleEditor
            value={config}
            onChange={(next) => {
              scheduleGate.clearRejection();
              setConfig(next);
            }}
            wsId={wsId}
            onValidityChange={scheduleGate.onValidityChange}
            // Same reason as the other two schedule dialogs: submit validates
            // over the network and then writes what it read going in, so an
            // edit landing inside that window would be discarded silently.
            disabled={submitting}
          />

          <div>
            <label className="text-caption font-medium text-muted-foreground">
              {t(($) => $.edit_trigger_dialog.label_field)}
            </label>
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder={t(($) => $.edit_trigger_dialog.label_placeholder)}
              className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-body outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          <div className="flex items-center justify-between gap-3">
            <span className="text-caption font-medium text-muted-foreground">
              {t(($) => $.edit_trigger_dialog.enabled_label)}
            </span>
            <Switch
              size="sm"
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={submitting}
              aria-label={t(($) => $.edit_trigger_dialog.enabled_label)}
            />
          </div>

          <div className="flex justify-end pt-1">
            <Button size="sm" onClick={handleSubmit} disabled={!canSubmit}>
              {submitting
                ? t(($) => $.edit_trigger_dialog.submitting)
                : t(($) => $.edit_trigger_dialog.submit)}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
