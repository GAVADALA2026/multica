package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func wakeupError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		writeError(w, 409, "source run is changing; retry registration")
		return
	}
	switch {
	case errors.Is(err, service.ErrWakeupConflict):
		writeError(w, 409, err.Error())
	case errors.Is(err, service.ErrWakeupInput):
		writeError(w, 400, err.Error())
	case errors.Is(err, service.ErrWakeupForbidden):
		writeError(w, 403, "wakeup permission denied")
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, 404, "wakeup not found")
	default:
		writeError(w, 500, "could not save wakeup")
	}
}

func (h *Handler) ListIssueWakeups(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.Queries.ListIssueWakeups(r.Context(), db.ListIssueWakeupsParams{IssueID: issue.ID, WorkspaceID: issue.WorkspaceID})
	if err != nil {
		wakeupError(w, err)
		return
	}
	if rows == nil {
		rows = []db.ListIssueWakeupsRow{}
	}
	workspaceID := uuidToString(issue.WorkspaceID)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), workspaceID)
	allowed, ok := h.accessibleAgentIDs(r.Context(), workspaceID, actorType, actorID, member.Role)
	if !ok {
		writeError(w, 500, "failed to resolve agent access")
		return
	}
	for i := range rows {
		if _, visible := allowed[uuidToString(rows[i].FilterAgentID)]; !visible {
			rows[i].FilterAgentName = pgtype.Text{}
		}
	}
	writeJSON(w, 200, rows)
}

func (h *Handler) ListWorkspaceWakeupSummaries(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), workspaceID)
	allowed, ok := h.accessibleAgentIDs(r.Context(), workspaceID, actorType, actorID, member.Role)
	if !ok {
		writeError(w, 500, "failed to resolve agent access")
		return
	}
	ids := make([]pgtype.UUID, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, parseUUID(id))
	}
	rows, err := h.Queries.ListWorkspaceWakeupSummaryRows(r.Context(), db.ListWorkspaceWakeupSummaryRowsParams{WorkspaceID: parseUUID(workspaceID), AgentIds: ids})
	if err != nil {
		wakeupError(w, err)
		return
	}
	if rows == nil {
		rows = []db.ListWorkspaceWakeupSummaryRowsRow{}
	}
	writeJSON(w, 200, rows)
}

func (h *Handler) CreateIssueWakeup(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var in service.WakeupInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, 400, "invalid wakeup body")
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(issue.WorkspaceID))
	if in.AgentID == "" && actorType == "agent" {
		in.AgentID = actorID
	}
	originator := h.invokeOriginatorFromRequest(r, actorType, actorID)
	if originator == "" {
		writeError(w, 403, "a human originator is required")
		return
	}
	member := parseUUID(originator)
	svc := service.IssueWakeupService{Tasks: h.TaskService}
	existingID := pgtype.UUID{}
	if rawID := chi.URLParam(r, "wakeupID"); rawID != "" {
		var valid bool
		existingID, valid = parseUUIDOrBadRequest(w, rawID, "wakeup id")
		if !valid {
			return
		}
	}
	if r.Method == http.MethodPut && !existingID.Valid {
		writeError(w, 400, "invalid wakeup id")
		return
	}
	result, err := svc.Save(r.Context(), issue.ID, member, h.wakeupSourceTaskID(r), existingID, in)
	if err != nil {
		wakeupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) DisableIssueWakeup(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "wakeupID"), "wakeup id")
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(issue.WorkspaceID))
	originator := h.invokeOriginatorFromRequest(r, actorType, actorID)
	if originator == "" {
		writeError(w, 403, "a human originator is required")
		return
	}
	member := parseUUID(originator)
	svc := service.IssueWakeupService{Tasks: h.TaskService}
	result, err := svc.Disable(r.Context(), issue.ID, id, member)
	if err != nil {
		wakeupError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (h *Handler) wakeupSourceTaskID(r *http.Request) pgtype.UUID {
	actorType, _ := h.resolveActor(r, requestUserID(r), h.resolveWorkspaceID(r))
	if actorType != "agent" {
		return pgtype.UUID{}
	}
	return h.commentSourceTaskID(r)
}

func (h *Handler) EnableIssueWakeup(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "wakeupID"), "wakeup id")
	if !ok {
		return
	}
	var in service.WakeupEnableInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, 400, "invalid enable body")
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(issue.WorkspaceID))
	originator := h.invokeOriginatorFromRequest(r, actorType, actorID)
	if originator == "" {
		writeError(w, 403, "a human originator is required")
		return
	}
	svc := service.IssueWakeupService{Tasks: h.TaskService}
	result, err := svc.Enable(r.Context(), issue.ID, parseUUID(originator), h.wakeupSourceTaskID(r), id, in)
	if err != nil {
		wakeupError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
