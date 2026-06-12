package main

import (
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	shortCodeLength      = 6
	maxShortCodeAttempts = 10
)

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	ShortCode string `json:"short_code"`
}

type statsResponse struct {
	ShortCode  string    `json:"short_code"`
	TargetURL  string    `json:"target_url"`
	ClickCount int       `json:"click_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type apiRootResponse struct {
	Service string `json:"service"`
	Health  string `json:"health"`
}

var errShortCodeExists = errors.New("short code already exists")
var errShortCodeNotFound = errors.New("short code not found")
var errShortCodeGenerationFailed = errors.New("failed to generate unique short code")

type urlStorage interface {
	Save(shortCode, targetURL string, userID int64) error
	Lookup(shortCode string) (string, error)
	IncrementClickCount(shortCode string) error
	GetStats(shortCode string, userID int64) (statsResponse, error)
	CreateUser(email, passwordHash string) (userRecord, error)
	GetUserByEmail(email string) (userRecord, error)
	GetUserByID(userID int64) (userRecord, error)
	StoreRefreshToken(userID int64, tokenHash string, expiresAt time.Time) error
	RotateRefreshToken(currentTokenHash, newTokenHash string, expiresAt time.Time) (int64, error)
	RevokeRefreshToken(tokenHash string) error
	ListOwnedLinks(userID int64) ([]ownedLinkResponse, error)
}

type server struct {
	storage urlStorage
	auth    *authManager
}

func newStorage() (urlStorage, func() error, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}

	store, err := newPostgresURLStore(databaseURL)
	if err != nil {
		return nil, nil, err
	}

	return store, store.Close, nil
}

func logRequest(r *http.Request) {
	log.Printf("received request: method=%s path=%s query=%s remote=%s\n", r.Method, r.URL.Path, r.URL.RawQuery, r.RemoteAddr)
}

func createShortCode(storage urlStorage, targetURL string, userID int64) (string, error) {
	for attempt := 0; attempt < maxShortCodeAttempts; attempt++ {
		shortCode, err := generateShortCode()
		if err != nil {
			return "", err
		}

		if err := storage.Save(shortCode, targetURL, userID); err == nil {
			return shortCode, nil
		} else if !errors.Is(err, errShortCodeExists) {
			return "", err
		}
	}

	return "", errShortCodeGenerationFailed
}

func generateShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	buf := make([]byte, shortCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	for i := range buf {
		buf[i] = charset[int(buf[i])%len(charset)]
	}

	return string(buf), nil
}

func normalizeTargetURL(rawURL string) (string, error) {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return "", err
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("unsupported URL scheme")
	}
	if parsedURL.Host == "" {
		return "", errors.New("missing URL host")
	}

	return parsedURL.String(), nil
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/" {
		writeJSONResponse(w, http.StatusOK, apiRootResponse{
			Service: "url-shortener-api",
			Health:  "/healthz",
		})
		return
	}

	if r.URL.Path != "/" {
		shortCode := strings.TrimPrefix(r.URL.Path, "/")
		if strings.Contains(shortCode, "/") {
			http.NotFound(w, r)
			return
		}

		targetURL, err := s.storage.Lookup(shortCode)
		if errors.Is(err, errShortCodeNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("failed to look up short code %q: %v\n", shortCode, err)
			http.Error(w, "failed to look up short code", http.StatusInternalServerError)
			return
		}
		if err := s.storage.IncrementClickCount(shortCode); err != nil {
			// Redirecting is still the primary behavior; analytics should not break it.
			log.Printf("failed to increment click count for short code %q: %v\n", shortCode, err)
		}

		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *server) handleShorten(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ensureSameOrigin(r); err != nil {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}

	user, ok := s.requireCurrentUser(w, r)
	if !ok {
		return
	}

	var req shortenRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONBodyError(w, err)
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	var err error
	req.URL, err = normalizeTargetURL(req.URL)
	if err != nil {
		http.Error(w, "url must be a valid http or https URL", http.StatusBadRequest)
		return
	}

	shortCode, err := createShortCode(s.storage, req.URL, user.ID)
	if err != nil {
		http.Error(w, "failed to generate short code", http.StatusInternalServerError)
		return
	}

	resp := shortenResponse{ShortCode: shortCode}

	writeJSONResponse(w, http.StatusOK, resp)
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := s.requireCurrentUser(w, r)
	if !ok {
		return
	}

	shortCode := strings.TrimPrefix(r.URL.Path, "/stats/")
	if shortCode == "" || shortCode == r.URL.Path || strings.Contains(shortCode, "/") {
		http.NotFound(w, r)
		return
	}

	stats, err := s.storage.GetStats(shortCode, user.ID)
	if errors.Is(err, errShortCodeNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, errForbidden) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err != nil {
		log.Printf("failed to load stats for short code %q: %v\n", shortCode, err)
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, http.StatusOK, stats)
}

func (s *server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/", s.handleRoot)
	r.Get("/healthz", s.handleHealthz)

	r.Route("/auth", func(r chi.Router) {
		r.Post("/signup", s.handleSignup)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Post("/refresh", s.handleRefresh)
	})

	r.Get("/me", s.handleMe)
	r.Get("/links", s.handleLinks)
	r.Post("/shorten", s.handleShorten)
	r.Get("/stats/{shortCode}", s.handleStats)
	r.Get("/{shortCode}", s.handleRoot)

	return r
}

func main() {
	if err := loadDotEnv(".env"); err != nil {
		log.Fatal(err)
	}

	storage, closeStorage, err := newStorage()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := closeStorage(); err != nil {
			log.Printf("failed to close storage: %v\n", err)
		}
	}()

	auth, err := newAuthManagerFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	srv := &server{storage: storage, auth: auth}

	addr := ":" + envOrDefault("PORT", "8081")
	log.Printf("using postgres storage\n")
	log.Printf("server listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}
