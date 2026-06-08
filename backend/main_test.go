package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubURLStore struct {
	savedShortCode       string
	savedTargetURL       string
	savedUserID          int64
	saveErr              error
	lookupTarget         string
	lookupErr            error
	incremented          []string
	incrementErr         error
	stats                statsResponse
	statsErr             error
	statsRequestedUserID int64
	createdUser          userRecord
	createUserErr        error
	userByEmail          userRecord
	userByEmailErr       error
	userByID             userRecord
	userByIDErr          error
	storedRefreshUserID  int64
	storedRefreshHash    string
	storedRefreshExpiry  time.Time
	storeRefreshErr      error
	rotatedCurrentHash   string
	rotatedNewHash       string
	rotatedExpiry        time.Time
	rotateUserID         int64
	rotateErr            error
	revokedTokens        []string
	revokeErr            error
	links                []ownedLinkResponse
	linksErr             error
}

func (s *stubURLStore) Save(shortCode, targetURL string, userID int64) error {
	s.savedShortCode = shortCode
	s.savedTargetURL = targetURL
	s.savedUserID = userID
	return s.saveErr
}

func (s *stubURLStore) Lookup(string) (string, error) {
	return s.lookupTarget, s.lookupErr
}

func (s *stubURLStore) IncrementClickCount(shortCode string) error {
	s.incremented = append(s.incremented, shortCode)
	return s.incrementErr
}

func (s *stubURLStore) GetStats(_ string, userID int64) (statsResponse, error) {
	s.statsRequestedUserID = userID
	return s.stats, s.statsErr
}

func (s *stubURLStore) CreateUser(_ string, _ string) (userRecord, error) {
	return s.createdUser, s.createUserErr
}

func (s *stubURLStore) GetUserByEmail(string) (userRecord, error) {
	return s.userByEmail, s.userByEmailErr
}

func (s *stubURLStore) GetUserByID(int64) (userRecord, error) {
	return s.userByID, s.userByIDErr
}

func (s *stubURLStore) StoreRefreshToken(userID int64, tokenHash string, expiresAt time.Time) error {
	s.storedRefreshUserID = userID
	s.storedRefreshHash = tokenHash
	s.storedRefreshExpiry = expiresAt
	return s.storeRefreshErr
}

func (s *stubURLStore) RotateRefreshToken(currentTokenHash, newTokenHash string, expiresAt time.Time) (int64, error) {
	s.rotatedCurrentHash = currentTokenHash
	s.rotatedNewHash = newTokenHash
	s.rotatedExpiry = expiresAt
	return s.rotateUserID, s.rotateErr
}

func (s *stubURLStore) RevokeRefreshToken(tokenHash string) error {
	s.revokedTokens = append(s.revokedTokens, tokenHash)
	return s.revokeErr
}

func (s *stubURLStore) ListOwnedLinks(int64) ([]ownedLinkResponse, error) {
	return s.links, s.linksErr
}

func newTestServer(t *testing.T, store *stubURLStore) *server {
	t.Helper()
	if store == nil {
		store = &stubURLStore{}
	}

	return &server{
		storage: store,
		auth: &authManager{
			accessSecret:  []byte("test-secret"),
			issuer:        "url-shortener-test",
			accessTTL:     15 * time.Minute,
			refreshTTL:    24 * time.Hour,
			secureCookies: false,
		},
	}
}

func addAccessCookie(t *testing.T, req *http.Request, srv *server, userID int64) {
	t.Helper()
	token, err := srv.auth.createAccessToken(userID)
	if err != nil {
		t.Fatalf("expected access token: %v", err)
	}

	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: token})
}

func hasCookie(rec *httptest.ResponseRecorder, cookieName string) bool {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == cookieName {
			return true
		}
	}

	return false
}

func TestHandleSignupCreatesUserAndSetsCookies(t *testing.T) {
	store := &stubURLStore{
		createdUser: userRecord{ID: 7, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", strings.NewReader(`{"email":"dev@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleSignup(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if !hasCookie(rec, accessCookieName) || !hasCookie(rec, refreshCookieName) {
		t.Fatalf("expected auth cookies to be set")
	}
	if store.storedRefreshUserID != 7 || store.storedRefreshHash == "" {
		t.Fatalf("expected refresh token to be stored, got user=%d hash=%q", store.storedRefreshUserID, store.storedRefreshHash)
	}
}

func TestHandleSignupReturnsConflictForDuplicateEmail(t *testing.T) {
	store := &stubURLStore{createUserErr: errUserExists}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", strings.NewReader(`{"email":"dev@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleSignup(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestHandleLoginSetsCookiesForValidCredentials(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	passwordHash, err := srv.auth.hashPassword("password123")
	if err != nil {
		t.Fatalf("expected password hash: %v", err)
	}

	store := &stubURLStore{
		userByEmail: userRecord{ID: 11, Email: "dev@example.com", PasswordHash: passwordHash, CreatedAt: time.Now().UTC()},
	}
	srv.storage = store
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"dev@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !hasCookie(rec, accessCookieName) || !hasCookie(rec, refreshCookieName) {
		t.Fatalf("expected auth cookies to be set")
	}
}

func TestHandleLoginRejectsWrongPassword(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	passwordHash, err := srv.auth.hashPassword("password123")
	if err != nil {
		t.Fatalf("expected password hash: %v", err)
	}

	store := &stubURLStore{
		userByEmail: userRecord{ID: 11, Email: "dev@example.com", PasswordHash: passwordHash, CreatedAt: time.Now().UTC()},
	}
	srv.storage = store
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"dev@example.com","password":"wrongpass"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHandleRefreshRotatesRefreshToken(t *testing.T) {
	store := &stubURLStore{rotateUserID: 42}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token"})
	rec := httptest.NewRecorder()

	srv.handleRefresh(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if store.rotatedCurrentHash == "" || store.rotatedNewHash == "" {
		t.Fatalf("expected refresh token rotation to be recorded")
	}
	if !hasCookie(rec, accessCookieName) || !hasCookie(rec, refreshCookieName) {
		t.Fatalf("expected refreshed cookies")
	}
}

func TestHandleMeReturnsCurrentUser(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp userResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if resp.Email != "dev@example.com" {
		t.Fatalf("expected email %q, got %q", "dev@example.com", resp.Email)
	}
}

func TestHandleLinksReturnsOwnedLinks(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
		links: []ownedLinkResponse{{
			ShortCode:  "abc123",
			TargetURL:  "https://example.com",
			ClickCount: 3,
			CreatedAt:  time.Now().UTC(),
		}},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHandleLinksReturnsEmptyArrayWhenUserHasNoLinks(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("expected empty JSON array, got %q", body)
	}
}

func TestHandleShortenRequiresAuth(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleShorten(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHandleShortenRejectsUnsupportedContentType(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "text/plain")
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleShorten(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status %d, got %d", http.StatusUnsupportedMediaType, rec.Code)
	}
}

func TestHandleShortenRejectsInvalidURL(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleShorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if store.savedTargetURL != "" {
		t.Fatalf("expected save not to be called, got %q", store.savedTargetURL)
	}
}

func TestHandleShortenRejectsUnknownFields(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com","extra":"value"}`))
	req.Header.Set("Content-Type", "application/json")
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleShorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleShortenCreatesShortCodeForValidURL(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleShorten(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if store.savedTargetURL != "https://example.com" {
		t.Fatalf("expected saved URL %q, got %q", "https://example.com", store.savedTargetURL)
	}
	if store.savedUserID != 42 {
		t.Fatalf("expected saved user ID %d, got %d", 42, store.savedUserID)
	}

	var resp shortenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if len(resp.ShortCode) != shortCodeLength {
		t.Fatalf("expected short code length %d, got %d", shortCodeLength, len(resp.ShortCode))
	}
}

func TestHandleRootReturnsAPIInfoForSlash(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}

	var resp apiRootResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if resp.Service != "url-shortener-api" || resp.Health != "/healthz" {
		t.Fatalf("unexpected API root response: %#v", resp)
	}
}

func TestHandleRootReturnsNotFoundForMissingShortCode(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{lookupErr: errShortCodeNotFound})
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestHandleRootReturnsInternalServerErrorForLookupFailure(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{lookupErr: errors.New("database down")})
	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestHandleRootRedirectsForKnownShortCode(t *testing.T) {
	store := &stubURLStore{lookupTarget: "https://example.com"}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "https://example.com" {
		t.Fatalf("expected redirect location %q, got %q", "https://example.com", location)
	}
	if len(store.incremented) != 1 || store.incremented[0] != "abc123" {
		t.Fatalf("expected click count increment for %q, got %#v", "abc123", store.incremented)
	}
}

func TestHandleRootStillRedirectsWhenClickCountIncrementFails(t *testing.T) {
	store := &stubURLStore{
		lookupTarget: "https://example.com",
		incrementErr: errors.New("analytics unavailable"),
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rec.Code)
	}
	if len(store.incremented) != 1 || store.incremented[0] != "abc123" {
		t.Fatalf("expected click count increment for %q, got %#v", "abc123", store.incremented)
	}
}

func TestHandleStatsRequiresAuth(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	req := httptest.NewRequest(http.MethodGet, "/stats/abc123", nil)
	rec := httptest.NewRecorder()

	srv.handleStats(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHandleStatsReturnsJSONForKnownShortCode(t *testing.T) {
	createdAt := time.Date(2026, time.March, 16, 8, 0, 0, 0, time.UTC)
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
		stats: statsResponse{
			ShortCode:  "abc123",
			TargetURL:  "https://example.com",
			ClickCount: 7,
			CreatedAt:  createdAt,
		},
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/stats/abc123", nil)
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}
	if store.statsRequestedUserID != 42 {
		t.Fatalf("expected owner ID %d, got %d", 42, store.statsRequestedUserID)
	}

	var resp statsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if resp.ShortCode != "abc123" || resp.ClickCount != 7 {
		t.Fatalf("unexpected stats response: %#v", resp)
	}
}

func TestHandleStatsReturnsForbiddenForOtherUsersLink(t *testing.T) {
	store := &stubURLStore{
		userByID: userRecord{ID: 42, Email: "dev@example.com", CreatedAt: time.Now().UTC()},
		statsErr: errForbidden,
	}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodGet, "/stats/abc123", nil)
	addAccessCookie(t, req, srv, 42)
	rec := httptest.NewRecorder()

	srv.handleStats(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestHandleHealthzReturnsOK(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ok\n" {
		t.Fatalf("expected body %q, got %q", "ok\n", rec.Body.String())
	}
}

func TestHandleLogoutClearsCookies(t *testing.T) {
	store := &stubURLStore{}
	srv := newTestServer(t, store)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token"})
	rec := httptest.NewRecorder()

	srv.handleLogout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if len(store.revokedTokens) != 1 {
		t.Fatalf("expected one revoked refresh token, got %#v", store.revokedTokens)
	}
	if !hasCookie(rec, accessCookieName) || !hasCookie(rec, refreshCookieName) {
		t.Fatalf("expected cleared auth cookies")
	}
}

func TestRoutesAllowConfiguredFrontendOriginPreflight(t *testing.T) {
	srv := newTestServer(t, &stubURLStore{})
	req := httptest.NewRequest(http.MethodOptions, "/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Fatalf("expected frontend origin CORS header, got %q", origin)
	}
	if credentials := rec.Header().Get("Access-Control-Allow-Credentials"); credentials != "true" {
		t.Fatalf("expected credentials CORS header, got %q", credentials)
	}
}
