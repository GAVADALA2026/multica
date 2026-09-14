package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChatSessionDeleteIndexMigrationsPreserveCoverageAndRollback(t *testing.T) {
	adminPool := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Uint32())
	schema := "migrate_chat_session_delete_idx_" + suffix
	schemaIdent := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaIdent); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+schemaIdent+" CASCADE"); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
	})

	pool := openTestPoolWithSearchPath(t, schema)
	for _, statement := range []string{
		`CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE agent_task_queue (
			id BIGSERIAL PRIMARY KEY,
			chat_session_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			session_id TEXT
		)`,
		`CREATE INDEX idx_agent_task_queue_chat_with_session_created_at
			ON agent_task_queue (chat_session_id, created_at DESC)
			WHERE chat_session_id IS NOT NULL
			  AND session_id IS NOT NULL`,
		`CREATE TABLE dingtalk_bot_identity (
			workspace_id UUID NOT NULL,
			installation_id UUID NOT NULL
		)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("apply fixture statement %q: %v", statement, err)
		}
	}

	versions := []string{
		"472_agent_task_queue_chat_session_index",
		"473_drop_agent_task_queue_chat_with_session_index",
		"474_dingtalk_bot_identity_workspace_index",
	}
	if err := runMigrations(ctx, pool, runOptions{
		Direction:             "up",
		Files:                 realMigrationFiles(t, versions, "up"),
		SchemaMigrationsTable: schema + ".schema_migrations",
		AdvisoryLockKey:       int64(rand.Uint64()&0x7fffffffffffffff) | 1,
		Hooks:                 hooksForDirection("up"),
	}); err != nil {
		t.Fatalf("apply chat-session delete index migrations: %v", err)
	}

	assertIndexValidity(t, pool, schema, "idx_agent_task_queue_chat_session", true)
	assertIndexExists(t, pool, schema, "idx_agent_task_queue_chat_with_session_created_at", false)
	assertIndexValidity(t, pool, schema, "idx_dingtalk_bot_identity_workspace", true)
	assertMigrationIndexShape(
		t, pool, schema, "idx_agent_task_queue_chat_session",
		"USING btree (chat_session_id, created_at DESC)",
		"chat_session_idISNOTNULL",
	)
	assertMigrationIndexShape(
		t, pool, schema, "idx_dingtalk_bot_identity_workspace",
		"USING btree (workspace_id)",
		"",
	)

	reversedVersions := []string{
		"474_dingtalk_bot_identity_workspace_index",
		"473_drop_agent_task_queue_chat_with_session_index",
		"472_agent_task_queue_chat_session_index",
	}
	if err := runMigrations(ctx, pool, runOptions{
		Direction:             "down",
		Files:                 realMigrationFiles(t, reversedVersions, "down"),
		SchemaMigrationsTable: schema + ".schema_migrations",
		AdvisoryLockKey:       int64(rand.Uint64()&0x7fffffffffffffff) | 1,
		Hooks:                 hooksForDirection("down"),
	}); err != nil {
		t.Fatalf("roll back chat-session delete index migrations: %v", err)
	}

	assertIndexExists(t, pool, schema, "idx_agent_task_queue_chat_session", false)
	assertIndexValidity(t, pool, schema, "idx_agent_task_queue_chat_with_session_created_at", true)
	assertIndexExists(t, pool, schema, "idx_dingtalk_bot_identity_workspace", false)
	assertMigrationIndexShape(
		t, pool, schema, "idx_agent_task_queue_chat_with_session_created_at",
		"USING btree (chat_session_id, created_at DESC)",
		"chat_session_idISNOTNULLANDsession_idISNOTNULL",
	)
}

func assertMigrationIndexShape(
	t *testing.T,
	pool *pgxpool.Pool,
	schema string,
	indexName string,
	wantKeys string,
	wantPredicate string,
) {
	t.Helper()
	var definition string
	var predicate string
	if err := pool.QueryRow(context.Background(), `
		SELECT pg_get_indexdef(indexrelid),
		       COALESCE(pg_get_expr(indpred, indrelid), '')
		FROM pg_index
		WHERE indexrelid = $1::regclass
	`, pgx.Identifier{schema, indexName}.Sanitize()).Scan(&definition, &predicate); err != nil {
		t.Fatalf("read index shape for %s.%s: %v", schema, indexName, err)
	}
	if !strings.Contains(definition, wantKeys) {
		t.Fatalf("index %s.%s definition = %q, want keys %q", schema, indexName, definition, wantKeys)
	}
	normalizedPredicate := strings.NewReplacer("(", "", ")", "", " ", "").Replace(predicate)
	if normalizedPredicate != wantPredicate {
		t.Fatalf("index %s.%s predicate = %q, want %q", schema, indexName, predicate, wantPredicate)
	}
}
