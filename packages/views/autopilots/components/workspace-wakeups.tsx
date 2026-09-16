"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bell, Clock3, AlertCircle } from "lucide-react";
import { toast } from "sonner";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import {
  workspaceWakeupsOptions,
  useDisableWorkspaceWakeups,
  useDisableIssueWakeup,
  useEnableIssueWakeup,
} from "@multica/core/issues/wakeups";
import type {
  WorkspaceWakeup,
  WorkspaceWakeupFilters,
} from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from "@multica/ui/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@multica/ui/components/ui/dialog";
import { AppLink } from "../../navigation";
import { useLocale, useT, useTimeAgo } from "../../i18n";
import { CollectionPageState } from "../../layout/collection-page";
import { ActorAvatar } from "../../common/actor-avatar";
import { TranscriptButton } from "../../common/task-transcript";
import { WakeupControl } from "../../issues/components/wakeup-control";
import {
  formatWakeupTime,
  isActiveWakeupRun,
  useWakeupText,
} from "../../issues/components/wakeup-presentation";

function WakeupListRow({
  row,
  selected,
  onSelect,
  busy,
}: {
  row: WorkspaceWakeup;
  selected: boolean;
  onSelect: () => void;
  busy: boolean;
}) {
  const { t } = useT("autopilots");
  const { t: ti } = useT("issues");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const locale = useLocale();
  const timeAgo = useTimeAgo();
  const text = useWakeupText();
  const disable = useDisableIssueWakeup(wsId, row.issue_id);
  const enable = useEnableIssueWakeup(wsId, row.issue_id);
  const Icon = row.kind === "event" ? Bell : Clock3;
  const next = row.issue_closed
    ? t(($) => $.wakeups.issue_closed)
    : !row.enabled
      ? row.disabled_at
        ? ti(($) => $.wakeups.disabled_state)
        : ti(($) => $.wakeups.completed)
      : row.kind === "event"
        ? ti(($) => $.wakeups.waiting_event)
        : row.next_fire_at
          ? formatWakeupTime(row.next_fire_at, locale, row.timezone)
          : "—";
  const status = row.task?.status;
  const labels: Record<string, string> = {
    queued: ti(($) => $.wakeups.run_queued),
    deferred: ti(($) => $.wakeups.run_deferred),
    dispatched: ti(($) => $.wakeups.run_dispatched),
    running: t(($) => $.wakeups.running),
    waiting_local_directory: ti(($) => $.wakeups.run_waiting_local_directory),
    completed: ti(($) => $.wakeups.completed),
    failed: ti(($) => $.wakeups.run_failed),
    cancelled: ti(($) => $.wakeups.run_cancelled),
  };
  return (
    <TableRow data-state={selected ? "selected" : undefined}>
      <TableCell className="w-10 pl-4">
        <Checkbox
          checked={selected}
          onCheckedChange={onSelect}
          disabled={busy || !row.can_manage || !row.enabled}
          aria-label={t(($) => $.wakeups.select_row, {
            issue: row.issue_identifier,
            agent: row.agent_name,
          })}
        />
      </TableCell>
      <TableCell className="max-w-72">
        <AppLink
          href={paths.issueDetail(row.issue_id)}
          className="block rounded-sm focus-visible:outline-2 focus-visible:outline-ring"
        >
          <span className="block truncate font-medium" title={row.issue_title}>
            {row.issue_title}
          </span>
          <span className="text-caption text-muted-foreground">
            {row.issue_identifier}
          </span>
        </AppLink>
      </TableCell>
      <TableCell className="max-w-44">
        <span className="flex items-center gap-2">
          <ActorAvatar
            actorType="agent"
            actorId={row.agent_id}
            name={row.agent_name}
            size="sm"
          />
          <span className="truncate">{row.agent_name}</span>
        </span>
      </TableCell>
      <TableCell className="max-w-64">
        <span
          className="flex items-center gap-1.5"
          title={row.event_types.map(text.eventName).join(", ")}
        >
          <Icon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">
            {row.kind === "event" ? text.trigger(row) : text.schedule(row)}
          </span>
        </span>
        <span className="block truncate text-caption text-muted-foreground">
          {row.kind === "event" ? text.schedule(row) : row.timezone}
        </span>
      </TableCell>
      <TableCell className="max-w-48">
        <span
          className="block truncate"
          title={
            row.next_fire_at && row.enabled
              ? `${new Date(row.next_fire_at).toLocaleString(locale, { timeZone: row.timezone })} · ${row.timezone}`
              : undefined
          }
        >
          {next}
        </span>
        {row.last_error && (
          <AppLink
            href={paths.issueDetail(row.issue_id)}
            className="text-caption text-destructive"
          >
            {ti(($) => $.wakeups.needs_attention)}
          </AppLink>
        )}
      </TableCell>
      <TableCell>
        {row.task ? (
          <div className="flex items-center gap-1">
            <div>
              <span
                className={
                  isActiveWakeupRun(status)
                    ? "text-primary"
                    : "text-muted-foreground"
                }
              >
                {labels[status ?? ""] ?? status}
              </span>
              <span className="block text-caption text-muted-foreground">
                {row.active_runs > 1
                  ? t(($) => $.wakeups.active_runs, { count: row.active_runs })
                  : timeAgo(
                      row.task.completed_at ??
                        row.task.started_at ??
                        row.task.created_at,
                    )}
              </span>
            </div>
            <TranscriptButton
              task={row.task}
              agentName={row.agent_name}
              title={ti(($) => $.wakeups.last_run)}
              isLive={isActiveWakeupRun(status)}
            />
          </div>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="pr-4">
        <div
          className="flex justify-end"
          title={
            !row.can_manage
              ? t(($) => $.wakeups.read_only)
              : row.issue_closed
                ? ti(($) => $.wakeups.closed_hint)
                : undefined
          }
        >
          <WakeupControl
            wakeup={row}
            task={row.task ?? undefined}
            closed={row.issue_closed}
            pending={
              busy || !row.can_manage || disable.isPending || enable.isPending
            }
            onDisable={() =>
              disable.mutate(row.id, {
                onError: () => toast.error(ti(($) => $.wakeups.disable_error)),
              })
            }
            onEnable={async (input = {}) => {
              await enable.mutateAsync({
                id: row.id,
                revision: row.revision ?? 0,
                ...input,
              });
            }}
          />
        </div>
      </TableCell>
    </TableRow>
  );
}

export function WorkspaceWakeups() {
  const { t } = useT("autopilots");
  const wsId = useWorkspaceId();
  const [filters, setFilters] = useState<WorkspaceWakeupFilters>({
    scope: "active",
    kind: "all",
    search: "",
    agent_id: "",
    offset: 0,
    limit: 50,
  });
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmation, setConfirmation] = useState<WorkspaceWakeup[]>([]);
  const [batchResult, setBatchResult] = useState<{
    failed: string[];
    succeeded: number;
  } | null>(null);
  const query = useQuery(workspaceWakeupsOptions(wsId, filters));
  const batch = useDisableWorkspaceWakeups(wsId);
  const rows = query.data?.items ?? [];
  const selectable = rows.filter((row) => row.enabled && row.can_manage);
  const picked = selectable.filter((row) => selected.has(row.id));
  const change = (patch: Partial<WorkspaceWakeupFilters>) => {
    setFilters((prev) => ({ ...prev, offset: 0, ...patch }));
    setSelected(new Set());
    setBatchResult(null);
  };
  const kinds = [
    { value: "all", label: t(($) => $.wakeups.all_triggers) },
    { value: "event", label: t(($) => $.wakeups.event) },
    { value: "at", label: t(($) => $.wakeups.at) },
    { value: "recurring", label: t(($) => $.wakeups.recurring) },
  ];
  const agents = [
    { value: "", label: t(($) => $.wakeups.all_agents) },
    ...(query.data?.agents ?? []).map((a) => ({ value: a.id, label: a.name })),
  ];
  return (
    <>
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2">
        <div
          className="flex gap-1"
          role="group"
          aria-label={t(($) => $.wakeups.scope)}
        >
          {(["active", "all", "disabled", "ended"] as const).map((scope) => (
            <Button
              key={scope}
              size="sm"
              variant={filters.scope === scope ? "secondary" : "ghost"}
              aria-pressed={filters.scope === scope}
              disabled={batch.isPending}
              onClick={() => change({ scope })}
            >
              {t(($) => $.wakeups.scopes[scope])}
              <span className="ml-1 text-muted-foreground tabular-nums">
                {query.data?.counts[scope] ?? "—"}
              </span>
            </Button>
          ))}
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <form
            onSubmit={(event) => {
              event.preventDefault();
              change({ search: search.trim() });
            }}
            className="flex items-center gap-1"
          >
            <Input
              type="search"
              className="h-8 w-48"
              value={search}
              maxLength={256}
              disabled={batch.isPending}
              onChange={(event) => {
                setSearch(event.target.value);
                if (!event.target.value) change({ search: "" });
              }}
              aria-label={t(($) => $.wakeups.search)}
              placeholder={t(($) => $.wakeups.search)}
            />
            <Button
              size="sm"
              variant="ghost"
              type="submit"
              disabled={batch.isPending}
            >
              {t(($) => $.wakeups.search_action)}
            </Button>
          </form>
          <Select
            items={kinds}
            value={filters.kind}
            disabled={batch.isPending}
            onValueChange={(kind) => {
              if (
                kind === "all" ||
                kind === "event" ||
                kind === "at" ||
                kind === "recurring"
              )
                change({ kind });
            }}
          >
            <SelectTrigger size="sm" aria-label={t(($) => $.wakeups.trigger)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {kinds.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            items={agents}
            value={filters.agent_id}
            disabled={batch.isPending}
            onValueChange={(agent_id) => {
              if (agent_id !== null) change({ agent_id });
            }}
          >
            <SelectTrigger
              size="sm"
              className="max-w-48"
              aria-label={t(($) => $.page.table.agent)}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {agents.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {batchResult && (
        <p
          role="status"
          className={`px-4 py-2 text-caption ${batchResult.failed.length ? "text-destructive" : "text-muted-foreground"}`}
        >
          {t(($) => $.wakeups.batch_result, {
            succeeded: batchResult.succeeded,
            failed: batchResult.failed.length,
          })}
        </p>
      )}
      {query.isError ? (
        <CollectionPageState
          icon={AlertCircle}
          tone="destructive"
          role="alert"
          title={t(($) => $.wakeups.load_error)}
          actions={
            <Button
              variant="outline"
              size="sm"
              onClick={() => void query.refetch()}
            >
              {t(($) => $.page.retry)}
            </Button>
          }
        />
      ) : query.isPending ? (
        <CollectionPageState
          icon={Clock3}
          title={t(($) => $.wakeups.loading)}
        />
      ) : !rows.length ? (
        <CollectionPageState icon={Bell} title={t(($) => $.wakeups.empty)} />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <Table className="min-w-[1050px]">
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">
                  <Checkbox
                    disabled={!selectable.length || batch.isPending}
                    checked={
                      picked.length > 0 && picked.length === selectable.length
                    }
                    indeterminate={
                      picked.length > 0 && picked.length < selectable.length
                    }
                    onCheckedChange={() =>
                      setSelected(
                        picked.length === selectable.length
                          ? new Set()
                          : new Set(selectable.map((row) => row.id)),
                      )
                    }
                    aria-label={t(($) => $.wakeups.select_page)}
                  />
                </TableHead>
                <TableHead>{t(($) => $.wakeups.issue)}</TableHead>
                <TableHead>{t(($) => $.page.table.agent)}</TableHead>
                <TableHead>{t(($) => $.wakeups.trigger)}</TableHead>
                <TableHead>{t(($) => $.wakeups.next)}</TableHead>
                <TableHead>{t(($) => $.page.table.last_run)}</TableHead>
                <TableHead className="pr-4 text-right">
                  {t(($) => $.wakeups.enabled)}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <WakeupListRow
                  key={row.id}
                  row={row}
                  busy={batch.isPending}
                  selected={selected.has(row.id)}
                  onSelect={() =>
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(row.id)) next.delete(row.id);
                      else next.add(row.id);
                      return next;
                    })
                  }
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <div className="mt-auto flex shrink-0 flex-wrap items-center gap-2 border-t px-4 py-2 text-caption text-muted-foreground">
        {picked.length > 0 && (
          <>
            <span>
              {t(($) => $.wakeups.selected, { count: picked.length })}
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={batch.isPending}
              onClick={() => setConfirmation(picked)}
            >
              {t(($) => $.wakeups.disable_selected)}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={batch.isPending}
              onClick={() => setSelected(new Set())}
            >
              {t(($) => $.wakeups.clear)}
            </Button>
          </>
        )}
        <span className="ml-auto tabular-nums">
          {t(($) => $.wakeups.results, {
            count: query.data?.total ?? 0,
            page: Math.floor(filters.offset / filters.limit) + 1,
          })}
        </span>
        <Button
          size="sm"
          variant="ghost"
          disabled={batch.isPending || !filters.offset}
          onClick={() =>
            change({ offset: Math.max(0, filters.offset - filters.limit) })
          }
        >
          {t(($) => $.wakeups.previous)}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={
            batch.isPending ||
            !query.data ||
            filters.offset + filters.limit >= query.data.total
          }
          onClick={() => change({ offset: filters.offset + filters.limit })}
        >
          {t(($) => $.wakeups.next_page)}
        </Button>
      </div>
      <Dialog
        open={confirmation.length > 0}
        onOpenChange={(open) => {
          if (!open && !batch.isPending) setConfirmation([]);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t(($) => $.wakeups.confirm_title, {
                count: confirmation.length,
              })}
            </DialogTitle>
            <DialogDescription>
              {t(($) => $.wakeups.confirm_body)}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={batch.isPending}
              onClick={() => setConfirmation([])}
            >
              {t(($) => $.wakeups.cancel)}
            </Button>
            <Button
              disabled={batch.isPending}
              onClick={async () => {
                const result = await batch.mutateAsync(confirmation);
                setBatchResult(result);
                setSelected(new Set(result.failed));
                setConfirmation([]);
              }}
            >
              {batch.isPending
                ? t(($) => $.wakeups.disabling)
                : t(($) => $.wakeups.confirm)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
