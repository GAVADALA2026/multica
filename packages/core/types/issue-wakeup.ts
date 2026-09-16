export interface IssueWakeup {
  id: string;
  issue_id: string;
  agent_id: string;
  agent_name: string;
  instruction: string;
  kind: "event" | "at" | "every" | "cron";
  mode: "once" | "continuous";
  event_types: string[];
  filter_agent_id: string | null;
  filter_task_id: string | null;
  interval_seconds: number | null;
  cron_expression: string | null;
  timezone: string;
  next_fire_at: string | null;
  enabled: boolean;
  revision?: number;
  disabled_at: string | null;
  last_task_id: string | null;
  last_error: string | null;
  filter_agent_name?: string | null;
  last_task_status?: string | null;
}

export type WakeupPreview = Pick<
  IssueWakeup,
  | "id"
  | "issue_id"
  | "agent_id"
  | "agent_name"
  | "kind"
  | "mode"
  | "event_types"
  | "filter_task_id"
  | "filter_agent_name"
  | "interval_seconds"
  | "cron_expression"
  | "timezone"
  | "next_fire_at"
>;
export interface IssueWakeupSummaryRow extends WakeupPreview {
  active_count: number;
  event_count: number;
}
