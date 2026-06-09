package main

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var migrationFilePattern = regexp.MustCompile(`^(\d+)_.+\.sql$`)

type migration struct {
	version int64
	name    string
	sql     string
}

func runMigrations(db *sql.DB) error {
	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		return err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`); err != nil {
		return err
	}

	for _, migration := range migrations {
		if err := applyMigration(db, migration); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, err
	}

	migrations := make([]migration, 0, len(entries))
	seenVersions := map[int64]string{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		matches := migrationFilePattern.FindStringSubmatch(name)
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}

		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version in %q: %w", name, err)
		}
		if existingName, exists := seenVersions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, existingName, name)
		}
		seenVersions[version] = name

		content, err := fs.ReadFile(source, filepath.ToSlash(filepath.Join("migrations", name)))
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migration{
			version: version,
			name:    name,
			sql:     strings.TrimSpace(string(content)),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func applyMigration(db *sql.DB, migration migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var alreadyApplied bool
	if err := tx.QueryRow(
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1);",
		migration.version,
	).Scan(&alreadyApplied); err != nil {
		return err
	}
	if alreadyApplied {
		return nil
	}

	if _, err := tx.Exec(migration.sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.name, err)
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2);",
		migration.version,
		migration.name,
	); err != nil {
		return err
	}

	return tx.Commit()
}
