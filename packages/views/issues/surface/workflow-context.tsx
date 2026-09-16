"use client";

import { createContext, useContext, useMemo } from "react";
import type {
  IssueWorkflowStatusNode,
  IssueStatusEntry,
  IssueTableGroupDescriptor,
  IssueTableFacetsResponse,
} from "@multica/core/types";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import {
  buildIssueStatusCatalog,
  normalizeIssueStatusCategory,
} from "@multica/core/issue-statuses/queries";

export const SurfaceWorkflowContext = createContext<{
  statuses?: IssueWorkflowStatusNode[];
  groups?: IssueTableGroupDescriptor[];
  facets?: IssueTableFacetsResponse;
}>({});

export function useSurfaceWorkflow() {
  return useContext(SurfaceWorkflowContext);
}

export function workflowColumnKey(group: IssueTableGroupDescriptor): string | undefined {
  if (group.value.kind === "workflow_status") {
    return group.value.workflow_status_id ?? group.value.status;
  }
  return group.value.kind === "status" ? group.value.status : undefined;
}

export function useSurfaceStatusCatalog(wsId: string) {
  const workspace = useIssueStatuses(wsId);
  const { statuses, groups, facets } = useSurfaceWorkflow();
  return useMemo(() => {
    if (!statuses) return workspace;
    const entries = new Map<string, IssueStatusEntry>(statuses.map((node) => [node.id, {
      ...node,
      workspace_id: wsId,
      key: node.id,
      category: normalizeIssueStatusCategory(node.phase) ?? "unstarted",
      is_system: false,
    }]));
    // Keep legacy saved-view filters readable alongside exact node filters.
    for (const legacy of workspace.statuses) if (!entries.has(legacy.key)) entries.set(legacy.key, legacy);
    const facetGroups: IssueTableGroupDescriptor[] = (facets?.facets ?? []).flatMap((facet) =>
      facet.values.flatMap((value) => value.status_node ? [{ key: value.key, count: value.count, value: value.status_node }] : []),
    );
    for (const group of [...(groups ?? []), ...facetGroups]) {
      for (const item of group.secondary_groups ?? [group]) {
        const value = item.value;
        if (value.kind !== "workflow_status") continue;
        if (!value.workflow_status_id) {
          const legacy = workspace.entryOf(value.status);
          if (legacy) entries.set(legacy.key, legacy);
          continue;
        }
        if (entries.has(value.workflow_status_id) && !facetGroups.includes(group)) continue;
        // Pinned definitions remain readable after project defaults change;
        // only archived nodes are unavailable as transition targets.
        entries.set(value.workflow_status_id, {
          id: value.workflow_status_id, workspace_id: wsId, key: value.workflow_status_id,
          name: value.name, description: "", category: normalizeIssueStatusCategory(value.phase ?? "unstarted") ?? "unstarted",
          color: value.color ?? "#808080", icon: value.icon, position: value.position ?? 0,
          is_system: false, archived_at: value.archived ? "archived" : null, created_at: "", updated_at: "",
        });
      }
    }
    return buildIssueStatusCatalog([...entries.values()].sort((a, b) => a.position - b.position));
  }, [groups, facets, statuses, workspace, wsId]);
}
