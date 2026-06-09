package main

import (
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsSortsByVersion(t *testing.T) {
	source := fstest.MapFS{
		"migrations/000002_second.sql": {Data: []byte("SELECT 2;")},
		"migrations/000001_first.sql":  {Data: []byte("SELECT 1;")},
	}

	migrations, err := loadMigrations(source)
	if err != nil {
		t.Fatalf("expected migrations to load: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}
	if migrations[0].version != 1 || migrations[0].name != "000001_first.sql" {
		t.Fatalf("expected first migration version 1, got %#v", migrations[0])
	}
	if migrations[1].version != 2 || migrations[1].name != "000002_second.sql" {
		t.Fatalf("expected second migration version 2, got %#v", migrations[1])
	}
}

func TestLoadMigrationsRejectsInvalidFilename(t *testing.T) {
	source := fstest.MapFS{
		"migrations/create_users.sql": {Data: []byte("SELECT 1;")},
	}

	if _, err := loadMigrations(source); err == nil {
		t.Fatalf("expected invalid filename error")
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	source := fstest.MapFS{
		"migrations/000001_first.sql": {Data: []byte("SELECT 1;")},
		"migrations/000001_other.sql": {Data: []byte("SELECT 2;")},
	}

	if _, err := loadMigrations(source); err == nil {
		t.Fatalf("expected duplicate migration version error")
	}
}
