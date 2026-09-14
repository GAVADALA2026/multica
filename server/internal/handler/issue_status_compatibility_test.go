package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// These are the installed Web/Desktop consumer's literal accepted buckets,
// deliberately independent of the server's current category constants.
var installedStatusBuckets = []string{"backlog", "todo", "in_progress", "in_review", "done", "blocked", "cancelled"}

func TestStatusCompatibilityMixedStorageGroupsAndPagination(t *testing.T) {
	for _, storage := range []string{"old", "mixed", "new"} {
		t.Run(storage, func(t *testing.T) {
			project := dbfx.Insert(t, "project", testutil.Cols{"workspace_id": testWorkspaceID, "title": "Compatibility " + storage})
			suffix := uuid.NewString()[:8]
			expectedWire := map[string]string{}
			expectedLifecycle := map[string]string{}
			for i, old := range installedStatusBuckets {
				category, _ := issuestatus.CategoryForBehavior(old)
				stored := category
				if storage == "old" || (storage == "mixed" && i%2 == 0) {
					stored = old
				}
				key := fmt.Sprintf("compat_%s_%d", suffix, i)
				cols := testutil.Cols{"workspace_id": testWorkspaceID, "key": key, "name": key, "category": stored, "color": "#123456"}
				if old == "cancelled" {
					cols["archived_at"] = testutil.Raw("now()")
				}
				dbfx.Insert(t, "issue_status", cols)
				for _, status := range []string{key, old} {
					dbfx.Issue(t, status, testutil.Cols{"status": status, "project_id": project})
					expectedWire[status] = issuestatus.WireCategory(status, category)
					expectedLifecycle[status] = category
				}
				keys, err := issuestatus.ExpandCategories(context.Background(), testHandler.Queries, parseUUID(testWorkspaceID), []string{category})
				if err != nil || !slices.Contains(keys, key) {
					t.Fatalf("category filter dropped %s: %v, %v", key, keys, err)
				}
			}
			query := statusCategoryQuery(project)
			for _, spec := range []issueTableGroupSpec{
				{Kind: "status_category"},
				{Kind: "compound", Primary: "project", Secondary: "status_category"},
				{Kind: "status_category", CategoryFormat: "lifecycle"},
				{Kind: "compound", Primary: "project", Secondary: "status_category", CategoryFormat: "lifecycle"},
				{Kind: "status"},
			} {
				t.Run(spec.Kind+"/"+spec.CategoryFormat, func(t *testing.T) {
					var groups issueTableGroupsResponse
					testutil.Call(t, testHandler.ListIssueTableGroups, newRequest(http.MethodPost, "/api/issues/table/groups", issueTableGroupsRequest{
						Query: query, Group: spec, Page: issueTablePageRequest{Limit: 100},
					})).Want(http.StatusOK).JSON(&groups)
					if groups.Total != 14 {
						t.Fatalf("total=%d, want 14", groups.Total)
					}
					cells := groups.Groups
					if spec.Kind == "compound" {
						if len(cells) != 1 {
							t.Fatalf("lanes=%d", len(cells))
						}
						cells = cells[0].SecondaryGroups
					}
					seen := map[string]bool{}
					for _, cell := range cells {
						legacy := spec.Kind != "status" && spec.CategoryFormat == ""
						if legacy && !slices.Contains(installedStatusBuckets, cell.Value.Status) {
							t.Fatalf("installed consumer hides %q", cell.Value.Status)
						}
						var cursor *string
						var count int64
						for page := 0; page < 20; page++ {
							var rows issueTableRowsResponse
							testutil.Call(t, testHandler.ListIssueTableRows, newRequest(http.MethodPost, "/api/issues/table/rows", issueTableRowsRequest{
								Query: query, Group: spec, GroupKey: &cell.Key, Page: issueTablePageRequest{Limit: 1, Cursor: cursor},
							})).Want(http.StatusOK).JSON(&rows)
							for _, row := range rows.Rows {
								status := row.Issue.Status
								if seen[status] {
									t.Fatalf("duplicate paged card: %s", status)
								}
								seen[status] = true
								want := status
								if legacy {
									want = expectedWire[status]
								} else if spec.Kind != "status" {
									want = expectedLifecycle[status]
								}
								if cell.Value.Status != want {
									t.Fatalf("card %s belongs to %s, got %s", status, want, cell.Value.Status)
								}
								if row.Issue.StatusCategory != expectedWire[status] {
									t.Fatalf("row category drift for %s: %s", status, row.Issue.StatusCategory)
								}
								count++
							}
							cursor = rows.NextCursor
							if cursor == nil {
								break
							}
							if page == 19 {
								t.Fatal("pagination did not terminate")
							}
						}
						if count != cell.Count {
							t.Fatalf("bucket %s: header=%d rows=%d", cell.Value.Status, cell.Count, count)
						}
					}
					if len(seen) != 14 {
						t.Fatalf("visible cards=%d, want 14", len(seen))
					}
				})
			}
			// A legacy client can show only In Review: it must not acquire the
			// In Progress/Blocked cards merely because all share lifecycle Started.
			var filtered issueTableGroupsResponse
			testutil.Call(t, testHandler.ListIssueTableGroups, newRequest(http.MethodPost, "/api/issues/table/groups", issueTableGroupsRequest{
				Query: query, Group: issueTableGroupSpec{Kind: "compound", Primary: "project", Secondary: "status_category", SecondaryValues: []string{"in_review"}}, Page: issueTablePageRequest{Limit: 1},
			})).Want(http.StatusOK).JSON(&filtered)
			if filtered.Total != 1 {
				t.Fatalf("visible In Review cards=%d, want 1", filtered.Total)
			}
		})
	}
}

func TestStatusCompatibilityCatalogWritesAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	key := "legacy_" + uuid.NewString()[:8]
	id := dbfx.Insert(t, "issue_status", testutil.Cols{"workspace_id": testWorkspaceID, "key": key, "name": key, "category": "in_review", "color": "#123456", "position": 99999})
	// Position allocation includes the old spelling in the same lifecycle.
	created, err := testHandler.Queries.CreateIssueStatusEntry(ctx, db.CreateIssueStatusEntryParams{
		WorkspaceID: parseUUID(testWorkspaceID), Key: key + "_new", Name: "New", Category: "started", Color: "#123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	dbfx.Cleanup(t, "DELETE FROM issue_status WHERE id=$1", created.ID)
	if created.Position != 100000 {
		t.Fatalf("new position=%v, want 100000", created.Position)
	}
	catalog, err := testHandler.Queries.ListIssueStatusEntries(ctx, db.ListIssueStatusEntriesParams{WorkspaceID: parseUUID(testWorkspaceID)})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, entry := range catalog {
		if normalizedStatusCategory(entry.Category) == "started" && !entry.IsSystem {
			ids = append(ids, uuid.UUID(entry.ID.Bytes).String())
		}
	}
	testutil.Call(t, testHandler.ReorderIssueStatuses, newRequest(http.MethodPost, "/api/issue-statuses/reorder", ReorderIssueStatusesRequest{Category: "in_review", IDs: ids})).Want(http.StatusOK)
	var stored string
	dbfx.QueryRow(t, "SELECT category FROM issue_status WHERE id=$1", id).Scan(&stored)
	if stored != "started" {
		t.Fatalf("reorder retained old category %s", stored)
	}
	foreign := dbfx.Workspace(t, "Other", "compat-"+uuid.NewString()[:8])
	foreignKey := "foreign_" + uuid.NewString()[:8]
	dbfx.Insert(t, "issue_status", testutil.Cols{"workspace_id": foreign, "key": foreignKey, "name": "Foreign", "category": "cancelled", "color": "#123456"})
	keys, err := issuestatus.ExpandCategories(ctx, testHandler.Queries, parseUUID(testWorkspaceID), []string{"closed"})
	if err != nil || slices.Contains(keys, foreignKey) {
		t.Fatalf("cross-workspace filter expansion: %v, %v", keys, err)
	}
	if got := issuestatus.Effective(ctx, testHandler.Queries, parseUUID(testWorkspaceID), foreignKey); got != foreignKey {
		t.Fatalf("cross-workspace terminal behavior: %s", got)
	}
}
