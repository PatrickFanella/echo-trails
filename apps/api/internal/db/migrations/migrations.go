package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Migration struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

func All() ([]Migration, error) {
	migrationsDir, err := directory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}

	type pair struct{ up, down string }
	pairs := map[string]pair{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, direction, ok := parseMigrationName(entry.Name())
		if !ok {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		key := version + "_" + name
		current := pairs[key]
		migrationPath := filepath.Join(migrationsDir, entry.Name())
		switch direction {
		case "up":
			current.up = migrationPath
		case "down":
			current.down = migrationPath
		default:
			return nil, fmt.Errorf("invalid migration direction %q", direction)
		}
		pairs[key] = current
	}

	keys := make([]string, 0, len(pairs))
	for key, pair := range pairs {
		if pair.up == "" || pair.down == "" {
			return nil, fmt.Errorf("migration %q must include both up and down files", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	migrations := make([]Migration, 0, len(keys))
	for _, key := range keys {
		version, name, _ := strings.Cut(key, "_")
		pair := pairs[key]
		migrations = append(migrations, Migration{Version: version, Name: name, UpPath: pair.up, DownPath: pair.down})
	}

	return migrations, nil
}

func Read(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

func directory() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate migrations package")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../db/migrations")), nil
}

func parseMigrationName(filename string) (version, name, direction string, ok bool) {
	trimmed := strings.TrimSuffix(filename, ".sql")
	withoutDirection, direction, found := strings.Cut(trimmed, ".")
	if !found || (direction != "up" && direction != "down") {
		return "", "", "", false
	}
	version, name, found = strings.Cut(withoutDirection, "_")
	if !found || version == "" || name == "" {
		return "", "", "", false
	}
	for _, r := range version {
		if r < '0' || r > '9' {
			return "", "", "", false
		}
	}
	return version, name, direction, true
}
