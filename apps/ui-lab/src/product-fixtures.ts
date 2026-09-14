import type {
  Issue,
  User,
  Workspace,
  MemberWithUser,
  StorageAdapter,
  IssueTableQuerySpec,
  Comment,
} from "@multica/core/types";
import { STATUS_ORDER } from "@multica/core/issues/config";
import { ApiClient } from "@multica/core/api";

const time = "2026-09-14T06:00:00Z";
export const workspace: Workspace = {
  id: "10000000-0000-4000-8000-000000000001",
  slug: "ui-lab",
  name: "Multica",
  description: "UI Lab fixture workspace",
  context: null,
  settings: {},
  repos: [],
  issue_prefix: "MUL",
  avatar_url: null,
  created_at: time,
  updated_at: time,
};
export const user: User = {
  id: "10000000-0000-4000-8000-000000000002",
  name: "Jiayuan",
  email: "designer@example.test",
  avatar_url: null,
  onboarded_at: time,
  onboarding_questionnaire: {},
  starter_content_state: "imported",
  language: "zh-Hans",
  profile_description: "",
  timezone: "Asia/Shanghai",
  created_at: time,
  updated_at: time,
};
export const members: MemberWithUser[] = [
  {
    id: "10000000-0000-4000-8000-000000000003",
    workspace_id: workspace.id,
    user_id: user.id,
    role: "owner",
    created_at: time,
    name: user.name,
    email: user.email,
    avatar_url: null,
  },
];
const titles = [
  "统一任务列表与详情页的视觉层次",
  "Improve keyboard navigation in the command menu",
  "优化智能体运行中的反馈与状态展示",
  "检查中英文混排，以及很长的任务标题在窄窗口中的截断表现",
  "为菜单和弹窗建立一致的圆角规则",
  "完善深色模式下的选中与悬停状态",
  "让空状态保持清晰、轻量",
];
export const issues: Issue[] = titles.map((title, index) => ({
  id: `20000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  workspace_id: workspace.id,
  number: 241 + index,
  identifier: `MUL-${241 + index}`,
  title,
  description:
    "## 目标\n\n让团队更容易找到当前最重要的信息。标题清晰、正文舒适，次要信息保持安静。\n\n## 验收标准\n\n- [x] 共享同一套语义颜色与圆角\n- [ ] 中英文混排与长标题表现自然\n- [ ] 选中状态在悬停时依然清晰\n\nBuild with intention. 让智能体和团队一起工作。",
  status: index < 3 ? "in_progress" : index < 6 ? "todo" : "done",
  priority: index % 2 ? "medium" : "high",
  assignee_type: "member",
  assignee_id: user.id,
  creator_type: "member",
  creator_id: user.id,
  parent_issue_id: null,
  project_id: null,
  position: index * 1000,
  stage: null,
  start_date: null,
  due_date: null,
  metadata: {},
  properties: {},
  labels: [],
  created_at: time,
  updated_at: time,
  revision: 1,
}));
export const memoryStorage = (): StorageAdapter => {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      values.set(key, value);
    },
    removeItem: (key) => {
      values.delete(key);
    },
    keys: () => [...values.keys()],
  };
};

// Only data is substituted. Views, hooks, stores and mutations are the production modules.
// There is deliberately no fallback to ApiClient's network implementation.
export function createFixtureApi() {
  let rows = structuredClone(issues);
  const comments: Comment[] = [
    {
      id: "30000000-0000-4000-8000-000000000001",
      issue_id: issues[0]!.id,
      author_type: "member",
      author_id: user.id,
      content: "先检查列表的密度，再比较详情页中的文字层级。",
      type: "comment",
      parent_id: null,
      reactions: [],
      attachments: [],
      created_at: time,
      updated_at: time,
      revision: 1,
      resolved_at: null,
      resolved_by_type: null,
      resolved_by_id: null,
    },
  ];
  const queryRows = (query: IssueTableQuerySpec) =>
    rows.filter((issue) => {
      const filters = query.filters;
      return (
        (!filters.statuses?.length ||
          filters.statuses.includes(issue.status)) &&
        (!filters.priorities?.length ||
          filters.priorities.includes(issue.priority)) &&
        (!query.search ||
          `${issue.identifier} ${issue.title}`
            .toLowerCase()
            .includes(query.search.toLowerCase())) &&
        (!filters.working_issue_ids ||
          filters.working_issue_ids.includes(issue.id)) &&
        (!("assignee_types" in query.scope) ||
          !query.scope.assignee_types?.length ||
          query.scope.assignee_types.includes(issue.assignee_type!))
      );
    });
  const handlers: Partial<ApiClient> = {
    getBaseUrl: () => "/ui-lab-fixtures",
    getMe: async () => user,
    listWorkspaces: async () => [workspace],
    getWorkspace: async () => workspace,
    listMembers: async () => members,
    listSquads: async () => [],
    listAgents: async () => [],
    listRuntimes: async () => [],
    listProjects: async () => ({ projects: [], total: 0 }),
    listIssues: async (params) => {
      const filtered = rows.filter(
        (issue) =>
          (!params?.statuses?.length ||
            params.statuses.includes(issue.status)) &&
          (!params?.ids || params.ids.includes(issue.id)) &&
          (!params?.priorities?.length ||
            params.priorities.includes(issue.priority)),
      );
      return {
        issues: filtered.slice(
          params?.offset ?? 0,
          (params?.offset ?? 0) + (params?.limit ?? 100),
        ),
        total: filtered.length,
      };
    },
    listIssueTableRows: async (request) => {
      const matched = queryRows(request.query);
      const group = request.group_key?.replace(/^status(?:_category)?:/, "");
      const branch = request.parent_id
        ? []
        : matched.filter((issue) => !group || issue.status === group);
      return {
        query_fingerprint: JSON.stringify(request.query),
        group_key: request.group_key,
        parent_id: request.parent_id,
        total: matched.length,
        branch_total: branch.length,
        rows: branch.map((issue) => ({ issue, direct_child_count: 0 })),
        next_cursor: null,
      };
    },
    listIssueTableGroups: async (request) => ({
      query_fingerprint: JSON.stringify(request.query),
      total: queryRows(request.query).length,
      groups: STATUS_ORDER.map((status) => ({
        key: `status:${status}`,
        value: { kind: "status", status },
        count: queryRows(request.query).filter(
          (issue) => issue.status === status,
        ).length,
      })),
      next_cursor: null,
    }),
    listIssueTableFacets: async (request) => {
      const matched = queryRows(request.query);
      return {
        query_fingerprint: JSON.stringify(request.query),
        total: matched.length,
        facets: request.facets.map((facet) => ({
          ...facet,
          values:
            facet.kind === "status"
              ? STATUS_ORDER.map((key) => ({
                  key,
                  count: matched.filter((issue) => issue.status === key).length,
                }))
              : [],
        })),
      };
    },
    getIssue: async (id) => {
      const issue = rows.find((row) => row.id === id || row.identifier === id);
      if (!issue) throw new Error(`Unknown fixture issue: ${id}`);
      return structuredClone(issue);
    },
    updateIssue: async (id, updates) => {
      const current = rows.find((row) => row.id === id);
      if (!current) throw new Error("Unknown fixture issue");
      const next = {
        ...current,
        ...updates,
        revision: (current.revision ?? 0) + 1,
      };
      rows = rows.map((row) => (row.id === id ? next : row));
      return structuredClone(next);
    },
    listChildIssues: async () => ({ issues: [] }),
    listChildrenByParents: async () => ({ issues: [] }),
    getChildIssueProgress: async () => ({ progress: [] }),
    listComments: async (id) =>
      comments.filter((comment) => comment.issue_id === id),
    listTimeline: async (id) =>
      comments
        .filter((comment) => comment.issue_id === id)
        .map((comment) => ({
          ...comment,
          type: "comment",
          actor_type: comment.author_type,
          actor_id: comment.author_id,
          actor_name: user.name,
        })),
    previewCommentTriggers: async () => ({ agents: [] }),
    createComment: async (issueId, content, _type, parentId) => {
      const comment: Comment = {
        id: crypto.randomUUID(),
        issue_id: issueId,
        author_type: "member",
        author_id: user.id,
        content,
        type: "comment",
        parent_id: parentId ?? null,
        reactions: [],
        attachments: [],
        created_at: time,
        updated_at: time,
        revision: 1,
        resolved_at: null,
        resolved_by_type: null,
        resolved_by_id: null,
      };
      comments.push(comment);
      return comment;
    },
    listTasksByIssue: async () => [],
    getActiveTasksForIssue: async () => ({ tasks: [] }),
    getAgentTaskSnapshot: async () => [],
    getWorkspaceWorkingAgents: async () => [],
    listGitHubInstallations: async () => ({
      installations: [],
      configured: false,
    }),
    listIssuePullRequests: async () => ({ pull_requests: [] }),
    listAttachments: async () => [],
    listIssueSubscribers: async () => [],
    getAssigneeFrequency: async () => [],
    listLabelsForIssue: async () => ({ labels: [] }),
    listLabels: async () => ({ labels: [], total: 0 }),
    listProperties: async () => ({ properties: [], total: 0 }),
    listQuickActions: async () => ({ quick_actions: [] }),
    listIssueStatuses: async () => ({
      statuses: STATUS_ORDER.map((key, index) => ({
        id: key,
        workspace_id: workspace.id,
        key,
        name: key,
        description: "",
        category: key,
        color: "#777777",
        is_system: true,
        position: index,
        archived_at: null,
        created_at: time,
        updated_at: time,
      })),
      categories: STATUS_ORDER,
      total: STATUS_ORDER.length,
    }),
    listPins: async () => [],
    listMyInvitations: async () => [],
    getInboxUnreadSummary: async () => [],
    getUnreadInboxCount: async () => ({ count: 0 }),
    listChatSessions: async () => [],
    listIssueViews: async () => [],
    getIssueViewPreference: async (params) => ({
      ...params,
      prefs: { hidden: [], order: [] },
      updated_at: time,
    }),
    listPluginInstallations: async () => ({ plugins: [] }),
    getWorkspaceSubscriptionSummary: async () => null,
    getIssueLimitUsage: async () => null,
  };
  return new Proxy(new ApiClient("/ui-lab-fixtures"), {
    get(_target, property: string) {
      const handler = handlers[property as keyof ApiClient];
      if (handler !== undefined) return handler;
      if (property === "then") return undefined;
      return () => {
        const message = `UI Lab 未提供此操作的样例数据：${property}`;
        console.warn(message);
        return Promise.reject(new Error(message));
      };
    },
  });
}
