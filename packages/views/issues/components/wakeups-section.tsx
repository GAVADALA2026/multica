"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bell, Clock3, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import {
  issueWakeupsOptions,
  useDisableIssueWakeup,
  issueTasksOptions,
} from "@multica/core/issues";
import type { AgentTask, IssueWakeup } from "@multica/core/types";
import { useCurrentWorkspace } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverTitle,
} from "@multica/ui/components/ui/popover";
import { TranscriptButton } from "../../common/task-transcript";
import { useT } from "../../i18n";
import {
  isCurrentWakeup,
  useWakeupText,
  wakeupRun,
} from "./wakeup-presentation";

function WakeupRow({
  wakeup,
  task,
  pending,
  onDisable,
}: {
  wakeup: IssueWakeup;
  task?: AgentTask;
  pending: boolean;
  onDisable: () => void;
}) {
  const { t } = useT("issues");
  const text = useWakeupText();
  const status = task?.status ?? wakeup.last_task_status;
  const canDisable =
    !wakeup.disabled_at &&
    (wakeup.enabled ||
      (["queued", "deferred"].includes(status ?? "") && !task?.started_at));
  const Icon = wakeup.kind === "event" ? Bell : Clock3;
  return (
    <div className="flex items-start gap-1">
      <Popover>
        <PopoverTrigger
          render={
            <button
              type="button"
              className="flex min-h-11 min-w-0 flex-1 items-start gap-2 rounded-md px-2 py-1.5 text-left hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            />
          }
        >
          <Icon
            className="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="min-w-0 flex-1 text-caption">
            <span className="block truncate font-medium">
              {text.trigger(wakeup)}
            </span>
            <span className="block truncate text-muted-foreground">
              {t(($) => $.wakeups.wake_agent, { agent: wakeup.agent_name })} ·{" "}
              {text.schedule(wakeup)}
            </span>
            {wakeup.last_error && (
              <span className="block text-destructive">
                {t(($) => $.wakeups.needs_attention)}
              </span>
            )}
          </span>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="max-h-[70dvh] w-80 max-w-[calc(100vw-2rem)] overflow-y-auto"
          keepMounted
        >
          <PopoverTitle>{text.trigger(wakeup)}</PopoverTitle>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.wakeups.wake_agent, { agent: wakeup.agent_name })} ·{" "}
            {text.schedule(wakeup)}
          </p>
          <p className="whitespace-pre-wrap break-words text-caption">
            {wakeup.instruction}
          </p>
          {wakeup.kind === "event" && (
            <p className="break-words text-caption text-muted-foreground">
              {wakeup.event_types.map(text.eventName).join(", ")}
            </p>
          )}
          {wakeup.filter_agent_id && (
            <p className="break-all text-caption text-muted-foreground">
              {t(($) => $.wakeups.source_agent)}:{" "}
              {wakeup.filter_agent_name ?? wakeup.filter_agent_id}
            </p>
          )}
          {wakeup.filter_task_id && (
            <p className="break-all text-caption text-muted-foreground">
              {t(($) => $.wakeups.source_run)}: {wakeup.filter_task_id}
            </p>
          )}
          {wakeup.next_fire_at && (
            <p className="text-caption text-muted-foreground">
              {new Date(wakeup.next_fire_at).toLocaleString(undefined, {
                timeZone: wakeup.timezone,
              })}{" "}
              · {wakeup.timezone}
            </p>
          )}
          {wakeup.last_error && (
            <p className="break-words text-caption text-destructive">
              {wakeup.last_error}
            </p>
          )}
          {task && (
            <div className="flex items-center gap-1 text-caption text-muted-foreground">
              <span>{t(($) => $.wakeups.last_run)}</span>
              <TranscriptButton
                task={task}
                agentName={wakeup.agent_name}
                title={t(($) => $.wakeups.last_run)}
              />
            </div>
          )}
        </PopoverContent>
      </Popover>
      {canDisable && (
        <Button
          variant="ghost"
          size="sm"
          className="min-h-11 shrink-0 px-2 text-caption text-muted-foreground"
          disabled={pending}
          aria-label={t(($) => $.wakeups.disable, { agent: wakeup.agent_name })}
          onClick={onDisable}
        >
          {t(($) => $.wakeups.turn_off)}
        </Button>
      )}
    </div>
  );
}

export function WakeupsSection({ issueId }: { issueId: string }) {
  const { t } = useT("issues");
  const workspaceId = useCurrentWorkspace()?.id ?? "";
  const [open, setOpen] = useState(true);
  const [historyOpen, setHistoryOpen] = useState(false);
  const {
    data = [],
    isError,
    refetch,
  } = useQuery(issueWakeupsOptions(workspaceId, issueId));
  const { data: tasks = [] } = useQuery(issueTasksOptions(issueId));
  const disable = useDisableIssueWakeup(workspaceId, issueId);
  if (!data.length && !isError) return null;
  const current = data.filter((w) => isCurrentWakeup(w, wakeupRun(w, tasks)));
  const history = data.filter((w) => !isCurrentWakeup(w, wakeupRun(w, tasks)));
  const row = (wakeup: IssueWakeup) => (
    <WakeupRow
      key={wakeup.id}
      wakeup={wakeup}
      task={wakeupRun(wakeup, tasks)}
      pending={disable.isPending}
      onDisable={() =>
        disable.mutate(wakeup.id, {
          onError: () => toast.error(t(($) => $.wakeups.disable_error)),
        })
      }
    />
  );
  return (
    <section>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="mb-1 flex min-h-9 w-full items-center gap-1 rounded-md px-2 py-1 text-caption font-medium hover:bg-accent/70 focus-visible:outline-2 focus-visible:outline-ring"
      >
        {t(($) => $.wakeups.title)}{" "}
        <span className="text-muted-foreground tabular-nums">
          {current.length}
        </span>
        <ChevronRight
          className={`size-3 text-muted-foreground ${open ? "rotate-90" : ""}`}
          aria-hidden="true"
        />
      </button>
      {open && (
        <div>
          {isError && (
            <button
              type="button"
              className="px-2 text-caption text-muted-foreground hover:text-foreground"
              onClick={() => void refetch()}
            >
              {t(($) => $.wakeups.retry)}
            </button>
          )}
          {current.map(row)}
          {history.length > 0 && (
            <>
              <button
                type="button"
                aria-expanded={historyOpen}
                onClick={() => setHistoryOpen(!historyOpen)}
                className="flex min-h-9 w-full items-center gap-1 rounded-md px-2 py-1 text-caption text-muted-foreground hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
              >
                <ChevronRight
                  className={`size-3 ${historyOpen ? "rotate-90" : ""}`}
                  aria-hidden="true"
                />
                {t(($) => $.wakeups.ended, { count: history.length })}
              </button>
              {historyOpen && history.map(row)}
            </>
          )}
        </div>
      )}
    </section>
  );
}
