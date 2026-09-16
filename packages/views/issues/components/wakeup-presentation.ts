"use client";

import type {
  AgentTask,
  IssueWakeup,
  WakeupPreview,
} from "@multica/core/types";
import { useLocale, useT } from "../../i18n";

export function isActiveWakeupRun(status?: string | null) {
  return (
    !!status &&
    [
      "queued",
      "deferred",
      "dispatched",
      "running",
      "waiting_local_directory",
    ].includes(status)
  );
}

export function wakeupRun(wakeup: IssueWakeup, tasks: readonly AgentTask[]) {
  return (
    tasks.find(
      (task) => task.wakeup_id === wakeup.id && isActiveWakeupRun(task.status),
    ) ?? tasks.find((task) => task.id === wakeup.last_task_id)
  );
}

export function isCurrentWakeup(wakeup: IssueWakeup, task?: AgentTask) {
  return (
    wakeup.enabled || isActiveWakeupRun(task?.status ?? wakeup.last_task_status)
  );
}

export function formatWakeupTime(
  value: string,
  locale: string,
  timezone = "UTC",
  now = new Date(),
) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  const day = new Intl.DateTimeFormat(locale, {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
  const sameDay = day.format(date) === day.format(now);
  const year = new Intl.DateTimeFormat(locale, {
    timeZone: timezone,
    year: "numeric",
  });
  return new Intl.DateTimeFormat(locale, {
    timeZone: timezone,
    hour: "2-digit",
    minute: "2-digit",
    ...(sameDay ? {} : { month: "short" as const, day: "numeric" as const }),
    ...(year.format(date) === year.format(now)
      ? {}
      : { year: "numeric" as const }),
  }).format(date);
}

export function useWakeupText() {
  const { t } = useT("issues");
  const locale = useLocale();
  const eventLabels: Record<string, string> = {
    "task.queued": t(($) => $.wakeups.run_queued),
    "task.dispatched": t(($) => $.wakeups.run_dispatched),
    "task.started": t(($) => $.wakeups.run_started),
    "task.deferred": t(($) => $.wakeups.run_deferred),
    "task.waiting_local_directory": t(
      ($) => $.wakeups.run_waiting_local_directory,
    ),
    "issue.updated": t(($) => $.wakeups.issue_updated),
    "issue.assignee_changed": t(($) => $.wakeups.assignee_changed),
    "issue.parent_changed": t(($) => $.wakeups.parent_changed),
    "issue.project_changed": t(($) => $.wakeups.project_changed),
    "issue.labels_changed": t(($) => $.wakeups.labels_changed),
    "issue.properties_changed": t(($) => $.wakeups.properties_changed),
    "issue.metadata_changed": t(($) => $.wakeups.metadata_changed),
    "comment.updated": t(($) => $.wakeups.comment_updated),
    "comment.deleted": t(($) => $.wakeups.comment_deleted),
    "comment.resolved": t(($) => $.wakeups.comment_resolved),
    "comment.unresolved": t(($) => $.wakeups.comment_unresolved),
    "reaction.added": t(($) => $.wakeups.reaction_added),
    "reaction.removed": t(($) => $.wakeups.reaction_removed),
    "attachment.attached": t(($) => $.wakeups.attachment_attached),
    "attachment.detached": t(($) => $.wakeups.attachment_detached),
    "task.completed": t(($) => $.wakeups.run_completed),
    "task.failed": t(($) => $.wakeups.run_failed),
    "task.cancelled": t(($) => $.wakeups.run_cancelled),
    "comment.created": t(($) => $.wakeups.comment_created),
    "issue.status_changed": t(($) => $.wakeups.status_changed),
  };

  const eventName = (event: string) => eventLabels[event] ?? event;
  const schedule = (w: WakeupPreview) =>
    w.kind === "every"
      ? t(($) => $.wakeups.every, { minutes: (w.interval_seconds ?? 0) / 60 })
      : w.kind === "cron"
        ? `${w.cron_expression} · ${w.timezone}`
        : w.mode === "once"
          ? t(($) => $.wakeups.once)
          : t(($) => $.wakeups.continuous);
  const trigger = (w: WakeupPreview) => {
    if (w.kind !== "event")
      return w.next_fire_at
        ? t(($) => $.wakeups.at_time, {
            time: formatWakeupTime(w.next_fire_at, locale, w.timezone),
          })
        : schedule(w);
    const source =
      w.filter_agent_name ||
      (w.filter_task_id ? w.filter_task_id.slice(0, 8) : "");
    const label = eventName(w.event_types[0] ?? "");
    return `${source ? `${source} · ` : ""}${label}${w.event_types.length > 1 ? ` +${w.event_types.length - 1}` : ""}`;
  };
  return { eventName, trigger, schedule };
}
