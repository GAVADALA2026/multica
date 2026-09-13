package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/testutil"
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
