/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { createStore } from "zustand/vanilla";
import { ViewStoreProvider } from "@multica/core/issues/stores/view-store-context";
import { viewStoreSlice, type IssueViewState } from "@multica/core/issues/stores/view-store";
import { renderWithI18n } from "../../test/i18n";
import type { IssueGroupBranches } from "../surface/use-issue-group-branches";
import { WorkflowBoardView } from "./workflow-board-view";

vi.mock("./board-view", () => ({ BoardView: ({ groupBranches, ownWorkflow, onCreateIssue }: any) => (
  <div data-testid="lane-board" data-own-workflow={ownWorkflow}>
    {groupBranches.descriptors.map((cell: any) => <button key={cell.key} onClick={() => onCreateIssue({ workflow_status_id: cell.value.workflow_status_id })}>{cell.value.name}</button>)}
  </div>
) }));
afterEach(cleanup);

const branches: IssueGroupBranches = {
  enabled: true, issues: [], pagination: {}, total: 57, isLoading: false, isRefreshing: false, isError: false,
  hasMoreGroups: false, isLoadingMoreGroups: false, loadMoreGroups: vi.fn(), retryGroups: vi.fn(),
  descriptors: ["Engineering", "Design"].map((name, i) => ({
    key: `workflow:${i}`, value: { kind: "workflow", workflow_id: String(i), name }, count: i ? 6 : 51,
    secondary_groups: [{ key: `opaque-${i}`, value: { kind: "workflow_status", workflow_id: String(i), workflow_status_id: `review-${i}`, name: `${name} review`, status: "in_review" }, count: i ? 6 : 51 }],
  })),
};
function renderBoard(data = branches) {
  const store = createStore<IssueViewState>()((set) => viewStoreSlice(set));
  const onCreateIssue = vi.fn();
  renderWithI18n(<ViewStoreProvider store={store}><WorkflowBoardView issues={[]} visibleStatuses={[]} hiddenStatuses={[]}
    onMoveIssue={vi.fn()} onCreateIssue={onCreateIssue} groupBranches={data} /> </ViewStoreProvider>);
  return { store, onCreateIssue };
}
describe("workflow board", () => {
  it("renders server lanes and counts before cards arrive, and collapses without changing filters", () => {
    const { store } = renderBoard();
    expect(screen.getAllByTestId("lane-board")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Engineering 51" }));
    expect(screen.getAllByTestId("lane-board")).toHaveLength(1);
    expect(store.getState().collapsedWorkflowLanes).toEqual(["workflow:0"]);
    expect(store.getState().statusFilters).toEqual([]);
    expect(screen.getByRole("button", { name: "Engineering 51" })).toHaveAttribute("aria-expanded", "false");
  });
  it("requires an explicit project choice while retaining the exact target node", () => {
    const { onCreateIssue } = renderBoard();
    fireEvent.click(screen.getByRole("button", { name: "Design review" }));
    expect(onCreateIssue).toHaveBeenCalledWith({ project_id: null, required_workflow_id: "1", require_project_choice: true, workflow_status_id: "review-1" });
    for (const board of screen.getAllByTestId("lane-board")) expect(board).toHaveAttribute("data-own-workflow", "true");
  });
  it("omits the lane header only after the server confirms there is a single workflow", () => {
    renderBoard({ ...branches, descriptors: branches.descriptors.slice(0, 1) });
    expect(screen.queryByRole("button", { name: "Engineering 51" })).toBeNull();
    expect(screen.getByTestId("lane-board")).toBeInTheDocument();
    cleanup();
    renderBoard({ ...branches, descriptors: branches.descriptors.slice(0, 1), hasMoreGroups: true });
    expect(screen.getByRole("button", { name: "Engineering 51" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show more workflows" }));
    expect(branches.loadMoreGroups).toHaveBeenCalledOnce();
  });
});
