"use client";

import { useMemo, type ComponentProps } from "react";
import { ChevronDown, ChevronRight, Workflow } from "lucide-react";
import type { IssueTableGroupDescriptor } from "@multica/core/types";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { Button } from "@multica/ui/components/ui/button";
import { BoardView } from "./board-view";
import { useT } from "../../i18n";
import type { IssueGroupBranches } from "../surface/use-issue-group-branches";
import { workflowLaneBranches } from "../utils/workflow-lanes";

type Props = ComponentProps<typeof BoardView>;
const EMPTY_STATUSES: NonNullable<Props["workflowStatuses"]> = [];

/** Each lane owns a DndContext: a drag can only target its own concrete nodes. */
function WorkflowLane({ lane, branches, ...props }: Props & {
  lane: IssueTableGroupDescriptor;
  branches: IssueGroupBranches;
}) {
  const statusFilters = useViewStore((s) => s.statusFilters);
  const laneBranches = useMemo(
    () => workflowLaneBranches(branches, lane, statusFilters),
    [branches, lane, statusFilters],
  );
  return <BoardView {...props} groupBranches={laneBranches} workflowStatuses={EMPTY_STATUSES} ownWorkflow
    onCreateIssue={(defaults) => props.onCreateIssue?.({
      ...defaults,
      // Explicit null overrides a remembered draft project. The create form
      // requires a choice, then resolves that project's effective workflow.
      project_id: props.projectId ?? null,
      required_workflow_id: lane.value.kind === "workflow" ? lane.value.workflow_id ?? undefined : undefined,
      require_project_choice: !props.projectId,
    })}
  />;
}

export function WorkflowBoardView(props: Props) {
  const { t } = useT("issues");
  const collapsed = useViewStore((s) => s.collapsedWorkflowLanes);
  const toggle = useViewStore((s) => s.toggleWorkflowLaneCollapsed);
  const branches = props.groupBranches;
  if (!branches) return null;
  const lanes = branches.descriptors.filter((lane) => lane.value.kind === "workflow");
  // An explicitly opened empty project still offers its active drop targets.
  if (lanes.length === 0 && !branches.isError && props.workflowStatuses?.length) {
    return <BoardView {...props} ownWorkflow />;
  }
  const single = lanes.length === 1 && !branches.hasMoreGroups;
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-auto">
      {lanes.map((lane) => {
        const isCollapsed = !single && collapsed.includes(lane.key);
        const name = lane.value.kind === "workflow" ? lane.value.name : "";
        return (
          <section key={lane.key} className={single ? "flex min-h-0 flex-1 flex-col" : "shrink-0 border-b border-border pb-3"}>
            {!single && (
              <button type="button" aria-label={`${name || t(($) => $.board.legacy_workflow)} ${lane.count}`} aria-expanded={!isCollapsed} onClick={() => toggle(lane.key)}
                className="flex w-full items-center gap-2 px-4 py-3 text-body font-medium hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                {isCollapsed ? <ChevronRight className="size-3.5" /> : <ChevronDown className="size-3.5" />}
                <Workflow className="size-4 text-muted-foreground" />
                <span className="truncate">{name || t(($) => $.board.legacy_workflow)}</span>
                <span className="text-caption font-normal tabular-nums text-muted-foreground">{lane.count}</span>
              </button>
            )}
            {!isCollapsed && <div className={single ? "flex min-h-0 flex-1" : "flex h-80 min-h-0"}>
              <WorkflowLane {...props} lane={lane} branches={branches} />
            </div>}
          </section>
        );
      })}
      {(branches.hasMoreGroups || branches.isError) && (
        <div className="flex shrink-0 justify-center p-3">
          <Button variant="ghost" disabled={branches.isLoadingMoreGroups}
            onClick={branches.isError ? branches.retryGroups : branches.loadMoreGroups}>
            {branches.isError ? t(($) => $.table.load_more_failed_retry) : t(($) => $.board.more_workflows)}
          </Button>
        </div>
      )}
    </div>
  );
}
