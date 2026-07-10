package migrations

import (
	"strings"
	"testing"
)

func TestAllMigrationsArePairedAndOrdered(t *testing.T) {
	migrations, err := All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one migration")
	}

	previous := ""
	for _, migration := range migrations {
		if migration.Version <= previous {
			t.Fatalf("migration versions are not strictly ordered: %q after %q", migration.Version, previous)
		}
		previous = migration.Version
		if migration.UpPath == "" || migration.DownPath == "" {
			t.Fatalf("migration %s is missing an up or down file", migration.Version)
		}
	}
}

func TestCoreSchemaMigrationIncludesRequiredTables(t *testing.T) {
	migrations, err := All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	upSQL, err := Read(migrations[0].UpPath)
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}

	requiredTables := []string{
		"users",
		"user_profiles",
		"sessions",
		"devices",
		"auth_identities",
		"consents",
		"places",
		"echoes",
		"replies",
		"media_assets",
		"moderation_actions",
		"analytics_events",
		"audit_logs",
		"jobs",
		"idempotency_keys",
	}
	for _, table := range requiredTables {
		needle := "CREATE TABLE " + table + " ("
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("core schema migration missing %s", needle)
		}
	}

	for _, required := range []string{"CREATE EXTENSION IF NOT EXISTS postgis;", "CREATE EXTENSION IF NOT EXISTS citext;", "geography(Point, 4326)"} {
		if !strings.Contains(upSQL, required) {
			t.Fatalf("core schema migration missing %q", required)
		}
	}
}

func TestDownMigrationDropsCoreTablesAndHonorsDependencyOrder(t *testing.T) {
	migrations, err := All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	downSQL, err := Read(migrations[0].DownPath)
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}

	for _, table := range []string{
		"users",
		"user_profiles",
		"sessions",
		"devices",
		"auth_identities",
		"consents",
		"places",
		"echoes",
		"replies",
		"media_assets",
		"moderation_actions",
		"analytics_events",
		"audit_logs",
		"jobs",
		"idempotency_keys",
	} {
		assertContains(t, downSQL, "DROP TABLE IF EXISTS "+table+";")
	}

	assertBefore(t, downSQL, "DROP TABLE IF EXISTS moderation_actions;", "DROP TABLE IF EXISTS reports;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS echo_links;", "DROP TABLE IF EXISTS replies;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS echo_links;", "DROP TABLE IF EXISTS echoes;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS user_profiles;", "DROP TABLE IF EXISTS media_assets;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS media_assets;", "DROP TABLE IF EXISTS replies;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS media_assets;", "DROP TABLE IF EXISTS echoes;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS echoes;", "DROP TABLE IF EXISTS places;")
	assertBefore(t, downSQL, "DROP TABLE IF EXISTS places;", "DROP TABLE IF EXISTS users;")
}

func TestReleaseMigrationsIncludeDurableAuthAndDiscoveryState(t *testing.T) {
	migrations, err := All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) < 4 {
		t.Fatalf("expected at least four migrations, got %d", len(migrations))
	}

	releaseSQL, err := Read(migrations[2].UpPath)
	if err != nil {
		t.Fatalf("read release migration: %v", err)
	}
	for _, required := range []string{
		"CREATE EXTENSION IF NOT EXISTS unaccent;",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm;",
		"CREATE TABLE creation_authorizations (",
		"CREATE TABLE echo_rank_snapshots (",
		"CREATE TABLE notification_preferences (",
		"Downers Grove Public Library",
		"Downers Grove North High School",
	} {
		assertContains(t, releaseSQL, required)
	}

	authSQL, err := Read(migrations[3].UpPath)
	if err != nil {
		t.Fatalf("read auth migration: %v", err)
	}
	for _, required := range []string{
		"CREATE TABLE email_auth_requests (",
		"ADD COLUMN access_expires_at timestamptz",
		"ADD COLUMN handle_changed_at timestamptz",
	} {
		assertContains(t, authSQL, required)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q", needle)
	}
}

func assertBefore(t *testing.T, haystack, first, second string) {
	t.Helper()
	firstIndex := strings.Index(haystack, first)
	secondIndex := strings.Index(haystack, second)
	if firstIndex == -1 || secondIndex == -1 {
		t.Fatalf("cannot compare order for %q and %q", first, second)
	}
	if firstIndex > secondIndex {
		t.Fatalf("expected %q before %q", first, second)
	}
}
