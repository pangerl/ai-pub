package migration

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteMigrationRunnerAppliesInitialSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	runner := NewRunner(db, "sqlite", os.DirFS("../.."))
	report, err := runner.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Applied) != 14 {
		t.Fatalf("expected 14 applied migrations, got %d", len(report.Applied))
	}

	assertForeignKeyTarget(t, db, "release_events", "deploy_record_id", "deploy_records")
	assertForeignKeyTarget(t, db, "deploy_target_logs", "deploy_record_id", "deploy_records")
	assertForeignKeyTarget(t, db, "release_requests", "deployment_target_id", "deployment_targets")
	assertForeignKeyTarget(t, db, "ssh_deployment_targets", "deployment_target_id", "deployment_targets")
	assertForeignKeyTarget(t, db, "k8s_deployment_targets", "deployment_target_id", "deployment_targets")
	assertForeignKeyTarget(t, db, "deployment_states", "deployment_target_id", "deployment_targets")

	report, err = runner.Run(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Pending) != 0 {
		t.Fatalf("expected no pending migrations, got %d", len(report.Pending))
	}
}

func assertForeignKeyTarget(t *testing.T, db *sql.DB, table, column, wantTarget string) {
	t.Helper()

	rows, err := db.Query("PRAGMA foreign_key_list(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, seq int
		var target, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatal(err)
		}
		if from == column {
			if target != wantTarget {
				t.Fatalf("%s.%s references %q, want %q", table, column, target, wantTarget)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("foreign key %s.%s not found", table, column)
}

func TestMySQLMigrationRunnerCanReadMigrationSet(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	runner := NewRunner(db, "mysql", os.DirFS("../.."))
	report, err := runner.Run(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Pending) != 13 {
		t.Fatalf("expected 13 pending mysql migrations, got %d", len(report.Pending))
	}
}

func TestSplitStatementsKeepsTriggerBody(t *testing.T) {
	statements := splitStatements(`
CREATE TABLE events (id INTEGER PRIMARY KEY);
CREATE TRIGGER events_after_insert
AFTER INSERT ON events
BEGIN
  UPDATE events SET id = id WHERE id = NEW.id;
END;
`)
	if len(statements) != 2 {
		t.Fatalf("expected 2 statements, got %#v", statements)
	}
	if statements[1] != "CREATE TRIGGER events_after_insert\nAFTER INSERT ON events\nBEGIN\n  UPDATE events SET id = id WHERE id = NEW.id;\nEND" {
		t.Fatalf("unexpected trigger statement: %q", statements[1])
	}
}

func TestSplitStatementsSkipsCommentOnlyMigration(t *testing.T) {
	statements := splitStatements("-- MySQL migration does not require a schema change; it is a no-op.\n")
	if len(statements) != 0 {
		t.Fatalf("expected no statements, got %#v", statements)
	}
}
