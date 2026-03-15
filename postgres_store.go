package main

import (
	"database/sql"
	"errors"

	_ "github.com/lib/pq"
)

type postgresURLStore struct {
	db *sql.DB
}

func newPostgresURLStore(databaseURL string) (*postgresURLStore, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &postgresURLStore{db: db}
	if err := store.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *postgresURLStore) ensureSchema() error {
	const query = `
	CREATE TABLE IF NOT EXISTS shortened_urls (
		short_code TEXT PRIMARY KEY,
		target_url TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	_, err := s.db.Exec(query)
	return err
}

func (s *postgresURLStore) Save(shortCode, targetURL string) error {
	const query = `
	INSERT INTO shortened_urls (short_code, target_url)
	VALUES ($1, $2)
	ON CONFLICT (short_code) DO NOTHING;`

	result, err := s.db.Exec(query, shortCode, targetURL)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errShortCodeExists
	}

	return nil
}

func (s *postgresURLStore) Lookup(shortCode string) (string, error) {
	const query = `
	SELECT target_url
	FROM shortened_urls
	WHERE short_code = $1;`

	var targetURL string
	err := s.db.QueryRow(query, shortCode).Scan(&targetURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errShortCodeNotFound
	}
	if err != nil {
		return "", err
	}

	return targetURL, nil
}

func (s *postgresURLStore) Close() error {
	return s.db.Close()
}
