package migrations

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMigrateUpAgainstPostgres(t *testing.T) {
	databaseURL := os.Getenv("ECHOTRAILS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHOTRAILS_TEST_DATABASE_URL to run the Postgres/PostGIS migrate-up integration test")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql is required for the Postgres/PostGIS migrate-up integration test")
	}

	repoRoot := findRepoRoot(t)
	runMigrationScript(t, repoRoot, "migrate-up.sh", databaseURL, nil)
	runMigrationScript(t, repoRoot, "migrate-down.sh", databaseURL, []string{"ECHOTRAILS_ALLOW_DESTRUCTIVE_MIGRATION_DOWN=1"})
}

func runMigrationScript(t *testing.T, repoRoot, scriptName, databaseURL string, extraEnv []string) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(repoRoot, "apps/api/scripts", scriptName))
	cmd.Env = append(os.Environ(), "DATABASE_URL="+databaseURL)
	cmd.Env = append(cmd.Env, extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", scriptName, err, string(output))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(filepath.Dir(dir))
		}
	}
	t.Fatalf("could not locate repository root from %s", wd)
	return ""
}
