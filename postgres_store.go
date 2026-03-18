package main

import (
	"database/sql"
	"errors"
	"time"

	pq "github.com/lib/pq"
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
	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`
		CREATE TABLE IF NOT EXISTS refresh_tokens (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id),
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`
		CREATE TABLE IF NOT EXISTS shortened_urls (
			short_code TEXT PRIMARY KEY,
			target_url TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			click_count INTEGER NOT NULL DEFAULT 0,
			user_id BIGINT REFERENCES users(id)
		);`,
		`
		ALTER TABLE shortened_urls
		ADD COLUMN IF NOT EXISTS click_count INTEGER NOT NULL DEFAULT 0;`,
		`
		ALTER TABLE shortened_urls
		ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id);`,
		`
		CREATE INDEX IF NOT EXISTS idx_shortened_urls_user_id
		ON shortened_urls (user_id);`,
		`
		CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
		ON refresh_tokens (user_id);`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func (s *postgresURLStore) Save(shortCode, targetURL string, userID int64) error {
	const query = `
	INSERT INTO shortened_urls (short_code, target_url, user_id)
	VALUES ($1, $2, $3)
	ON CONFLICT (short_code) DO NOTHING;`

	result, err := s.db.Exec(query, shortCode, targetURL, userID)
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

func (s *postgresURLStore) IncrementClickCount(shortCode string) error {
	const query = `
	UPDATE shortened_urls
	SET click_count = click_count + 1
	WHERE short_code = $1;`

	result, err := s.db.Exec(query, shortCode)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errShortCodeNotFound
	}

	return nil
}

func (s *postgresURLStore) GetStats(shortCode string, userID int64) (statsResponse, error) {
	const query = `
	SELECT user_id, target_url, click_count, created_at
	FROM shortened_urls
	WHERE short_code = $1;`

	var (
		ownerID   sql.NullInt64
		targetURL string
		stats     statsResponse
	)

	err := s.db.QueryRow(query, shortCode).Scan(
		&ownerID,
		&targetURL,
		&stats.ClickCount,
		&stats.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return statsResponse{}, errShortCodeNotFound
	}
	if err != nil {
		return statsResponse{}, err
	}
	if !ownerID.Valid {
		return statsResponse{}, errShortCodeNotFound
	}
	if ownerID.Int64 != userID {
		return statsResponse{}, errForbidden
	}

	stats.ShortCode = shortCode
	stats.TargetURL = targetURL
	return stats, nil
}

func (s *postgresURLStore) CreateUser(email, passwordHash string) (userRecord, error) {
	const query = `
	INSERT INTO users (email, password_hash)
	VALUES ($1, $2)
	RETURNING id, email, password_hash, created_at;`

	var user userRecord
	err := s.db.QueryRow(query, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err == nil {
		return user, nil
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return userRecord{}, errUserExists
	}

	return userRecord{}, err
}

func (s *postgresURLStore) GetUserByEmail(email string) (userRecord, error) {
	const query = `
	SELECT id, email, password_hash, created_at
	FROM users
	WHERE email = $1;`

	var user userRecord
	err := s.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return userRecord{}, errUserNotFound
	}
	if err != nil {
		return userRecord{}, err
	}

	return user, nil
}

func (s *postgresURLStore) GetUserByID(userID int64) (userRecord, error) {
	const query = `
	SELECT id, email, password_hash, created_at
	FROM users
	WHERE id = $1;`

	var user userRecord
	err := s.db.QueryRow(query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return userRecord{}, errUserNotFound
	}
	if err != nil {
		return userRecord{}, err
	}

	return user, nil
}

func (s *postgresURLStore) StoreRefreshToken(userID int64, tokenHash string, expiresAt time.Time) error {
	const query = `
	INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
	VALUES ($1, $2, $3);`

	_, err := s.db.Exec(query, userID, tokenHash, expiresAt)
	return err
}

func (s *postgresURLStore) RotateRefreshToken(currentTokenHash, newTokenHash string, expiresAt time.Time) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const selectQuery = `
	SELECT user_id, expires_at, revoked_at
	FROM refresh_tokens
	WHERE token_hash = $1
	FOR UPDATE;`

	var (
		userID     int64
		currentExp time.Time
		revokedAt  sql.NullTime
	)

	err = tx.QueryRow(selectQuery, currentTokenHash).Scan(&userID, &currentExp, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errInvalidRefreshToken
	}
	if err != nil {
		return 0, err
	}
	if revokedAt.Valid || time.Now().UTC().After(currentExp) {
		return 0, errInvalidRefreshToken
	}

	const revokeQuery = `
	UPDATE refresh_tokens
	SET revoked_at = NOW()
	WHERE token_hash = $1;`
	if _, err := tx.Exec(revokeQuery, currentTokenHash); err != nil {
		return 0, err
	}

	const insertQuery = `
	INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
	VALUES ($1, $2, $3);`
	if _, err := tx.Exec(insertQuery, userID, newTokenHash, expiresAt); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *postgresURLStore) RevokeRefreshToken(tokenHash string) error {
	const query = `
	UPDATE refresh_tokens
	SET revoked_at = NOW()
	WHERE token_hash = $1
	  AND revoked_at IS NULL;`

	_, err := s.db.Exec(query, tokenHash)
	return err
}

func (s *postgresURLStore) ListOwnedLinks(userID int64) ([]ownedLinkResponse, error) {
	const query = `
	SELECT short_code, target_url, click_count, created_at
	FROM shortened_urls
	WHERE user_id = $1
	ORDER BY created_at DESC;`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []ownedLinkResponse
	for rows.Next() {
		var link ownedLinkResponse
		if err := rows.Scan(
			&link.ShortCode,
			&link.TargetURL,
			&link.ClickCount,
			&link.CreatedAt,
		); err != nil {
			return nil, err
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return links, nil
}

func (s *postgresURLStore) Close() error {
	return s.db.Close()
}
