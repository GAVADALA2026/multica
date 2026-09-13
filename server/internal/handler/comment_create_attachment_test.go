package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A comment and the attachments it was posted with are one database outcome
// (#8296 review). Committing the comment first left a window in which it could
// gain a reply and be deleted into a tombstone, and the link would then bind
// the uploaded objects to that placeholder — invisible in the timeline, and
// missed by the prune's storage cleanup.

// failLinkAttachmentsTxStarter begins real transactions whose
// LinkAttachmentsToComment statement fails, standing in for any failure
// between creating the comment and linking its attachments.
type failLinkAttachmentsTxStarter struct {
	delegate *pgxpool.Pool
}

type failLinkAttachmentsTx struct {
	pgx.Tx
}

func (s *failLinkAttachmentsTxStarter) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.delegate.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return failLinkAttachmentsTx{Tx: tx}, nil
}

func (t failLinkAttachmentsTx) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(query, "-- name: LinkAttachmentsToComment") {
		return pgconn.CommandTag{}, errors.New("injected attachment link failure")
	}
	return t.Tx.Exec(ctx, query, args...)
}

func createCommentWithAttachment(t *testing.T, h *Handler, issueID, attachmentID string) *testutil.Response {
	t.Helper()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", map[string]any{
		"content":        "see the screenshot",
		"attachment_ids": []string{attachmentID},
	})
	return testutil.Call(t, h.CreateComment, withURLParam(req, "id", issueID))
}

func unlinkedIssueAttachment(t *testing.T, issueID string) string {
	t.Helper()
	return dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": issueID,
		"uploader_type": "member", "uploader_id": testUserID,
		"filename": "shot.png", "url": "https://example.test/shot.png",
		"content_type": "image/png", "size_bytes": 1,
	})
}

func TestCreateCommentLinksItsAttachmentsInOneTransaction(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID := dbfx.Issue(t, "comment with attachments")
	attachmentID := unlinkedIssueAttachment(t, issueID)

	var resp CommentResponse
	createCommentWithAttachment(t, testHandler, issueID, attachmentID).Want(http.StatusCreated).JSON(&resp)
	if len(resp.Attachments) != 1 || resp.Attachments[0].ID != attachmentID {
		t.Fatalf("response attachments = %+v, want the posted one", resp.Attachments)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE id = $1 AND comment_id = $2`, attachmentID, resp.ID); n != 1 {
		t.Fatalf("attachment linked rows = %d, want 1", n)
	}
}

// A link that fails takes the comment with it: no comment may exist whose
// attachments were never bound.
func TestCreateCommentRollsBackWhenAttachmentLinkFails(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID := dbfx.Issue(t, "comment attachment link failure")
	attachmentID := unlinkedIssueAttachment(t, issueID)

	h := *testHandler
	h.TxStarter = &failLinkAttachmentsTxStarter{delegate: testPool}
	createCommentWithAttachment(t, &h, issueID, attachmentID).Want(http.StatusInternalServerError)

	if n := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID); n != 0 {
		t.Fatalf("issue has %d comments after a failed link, want none", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE id = $1 AND comment_id IS NULL`, attachmentID); n != 1 {
		t.Fatalf("attachment was left linked after the rollback")
	}
}

// Every mutation here takes its owners first — the issue, then the comment,
// then the attachment. A comment created with an attachment takes the issue in
// its CreateComment statement and only then locks the attachment rows, so it
// cannot close a cycle with the two writers that reach an attachment through
// its issue: DeleteAttachment (issue, then the row) and issue teardown (issue,
// then the rows its cascade removes). The holders below play each of those one
// statement at a time.
func TestCreateCommentWithAttachmentFollowsOwnerFirstLockOrder(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	tests := []struct {
		name         string
		holdSQL      string
		wantCode     int
		wantComments int
		wantLinked   int
	}{
		{
			// The attachment is gone by the time the create proceeds: refuse
			// before any comment exists rather than commit one without it.
			name:         "attachment deleted while the create waits",
			holdSQL:      `DELETE FROM attachment WHERE id = $1`,
			wantCode:     http.StatusConflict,
			wantComments: 0,
			wantLinked:   0,
		},
		{
			// The holder took the issue and released it without touching the
			// attachment: the create proceeds and links it.
			name:         "attachment kept",
			holdSQL:      `SELECT id FROM attachment WHERE id = $1`,
			wantCode:     http.StatusCreated,
			wantComments: 1,
			wantLinked:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			issueID := dbfx.Issue(t, "comment attachment vs attachment delete")
			attachmentID := unlinkedIssueAttachment(t, issueID)

			holder, err := testPool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin holder: %v", err)
			}
			defer holder.Rollback(context.Background())
			// DeleteAttachment's own order: the owner issue, then the row. The
			// mode matches the issue bump its delete statement performs, which
			// is what a concurrent create contends with.
			if _, err := holder.Exec(ctx, `SELECT id FROM issue WHERE id = $1 FOR NO KEY UPDATE`, issueID); err != nil {
				t.Fatalf("hold issue: %v", err)
			}
			if _, err := holder.Exec(ctx, tt.holdSQL, attachmentID); err != nil {
				t.Fatalf("hold attachment: %v", err)
			}

			var got *testutil.Response
			done := make(chan error, 1)
			go func() {
				got = createCommentWithAttachment(t, testHandler, issueID, attachmentID)
				done <- nil
			}()
			// The create waits for the issue while holding no attachment lock.
			waitForCommentMutationLock(t, "CreateComment", done)

			holdErr := holder.Commit(ctx)
			<-done
			if holdErr != nil {
				t.Fatalf("attachment delete deadlocked or failed: %v", holdErr)
			}
			got.Want(tt.wantCode)
			if n := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID); n != tt.wantComments {
				t.Fatalf("issue has %d comments, want %d", n, tt.wantComments)
			}
			if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE id = $1 AND comment_id IS NOT NULL`, attachmentID); n != tt.wantLinked {
				t.Fatalf("attachment linked rows = %d, want %d", n, tt.wantLinked)
			}
		})
	}
}

// The symmetric side: issue teardown holds the issue and then reaches the
// attachment through the issue_id cascade. A create that took the attachment
// first would deadlock with it; taking the issue first means the create simply
// waits, and then finds the issue gone.
func TestCreateCommentWithAttachmentDoesNotDeadlockWithIssueTeardown(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issueID := dbfx.Issue(t, "comment attachment vs issue teardown")
	attachmentID := unlinkedIssueAttachment(t, issueID)

	teardown, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin issue teardown: %v", err)
	}
	defer teardown.Rollback(context.Background())
	qtx := testHandler.Queries.WithTx(teardown)
	if _, err := qtx.LockIssueForDelete(ctx, db.LockIssueForDeleteParams{
		ID:          parseUUID(issueID),
		WorkspaceID: parseUUID(testWorkspaceID),
	}); err != nil {
		t.Fatalf("lock issue for teardown: %v", err)
	}

	var got *testutil.Response
	done := make(chan error, 1)
	go func() {
		got = createCommentWithAttachment(t, testHandler, issueID, attachmentID)
		done <- nil
	}()
	waitForCommentMutationLock(t, "CreateComment", done)

	// The cascade reaches the attachment the blocked create asked for.
	deleteErr := qtx.DeleteIssue(ctx, db.DeleteIssueParams{
		ID:          parseUUID(issueID),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if deleteErr == nil {
		deleteErr = teardown.Commit(ctx)
	}
	<-done
	if deleteErr != nil {
		t.Fatalf("issue teardown deadlocked or failed: %v", deleteErr)
	}
	got.Want(http.StatusNotFound)
	if n := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID); n != 0 {
		t.Fatalf("deleted issue kept %d comments", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE id = $1`, attachmentID); n != 0 {
		t.Fatalf("deleted issue kept its attachment")
	}
}

// DeleteAttachment itself must take the issue before the attachment row, so it
// queues behind issue teardown instead of deadlocking with it.
func TestDeleteAttachmentTakesTheIssueBeforeTheRow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issueID := dbfx.Issue(t, "attachment delete vs issue lock")
	attachmentID := unlinkedIssueAttachment(t, issueID)

	holder, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(ctx, `SELECT id FROM issue WHERE id = $1 FOR UPDATE`, issueID); err != nil {
		t.Fatalf("hold issue: %v", err)
	}

	var got *testutil.Response
	done := make(chan error, 1)
	go func() {
		req := newRequest(http.MethodDelete, "/api/attachments/"+attachmentID, nil)
		got = testutil.Call(t, testHandler.DeleteAttachment, withURLParam(req, "id", attachmentID))
		done <- nil
	}()
	waitForCommentMutationLock(t, "LockIssueForAttachmentWrite", done)

	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("release issue: %v", err)
	}
	<-done
	got.Want(http.StatusNoContent)
	if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE id = $1`, attachmentID); n != 0 {
		t.Fatalf("attachment survived its delete")
	}
}
