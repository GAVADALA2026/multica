import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api";
import { issueKeys } from "./queries";

export function workspaceWakeupSummariesOptions(workspaceId: string) {
  return queryOptions({
    queryKey: ["issue-wakeup-summaries", workspaceId],
    queryFn: () => api.listIssueWakeupSummaries(),
    enabled: !!workspaceId,
    staleTime: 10_000,
  });
}

export function issueWakeupsOptions(workspaceId: string, issueId: string) {
  return queryOptions({
    queryKey: ["issue-wakeups", workspaceId, issueId],
    queryFn: () => api.listIssueWakeups(issueId),
    enabled: !!workspaceId && !!issueId,
    refetchInterval: 10_000,
  });
}

export function useDisableIssueWakeup(workspaceId: string, issueId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.disableIssueWakeup(issueId, id),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({
          queryKey: issueWakeupsOptions(workspaceId, issueId).queryKey,
        }),
        client.invalidateQueries({
          queryKey: workspaceWakeupSummariesOptions(workspaceId).queryKey,
        }),
      ]);
    },
  });
}

export function useEnableIssueWakeup(workspaceId: string, issueId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...input
    }: {
      id: string;
      revision: number;
      at?: string;
      rearm?: boolean;
    }) => api.enableIssueWakeup(issueId, id, input),
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({
          queryKey: issueWakeupsOptions(workspaceId, issueId).queryKey,
        }),
        client.invalidateQueries({
          queryKey: workspaceWakeupSummariesOptions(workspaceId).queryKey,
        }),
        client.invalidateQueries({ queryKey: issueKeys.tasks(issueId) }),
      ]);
    },
  });
}
