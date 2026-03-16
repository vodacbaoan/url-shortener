package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const shortCodeLength = 6

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	ShortCode string `json:"short_code"`
}

var errShortCodeExists = errors.New("short code already exists")
var errShortCodeNotFound = errors.New("short code not found")

type urlStorage interface {
	Save(shortCode, targetURL string) error
	Lookup(shortCode string) (string, error)
	IncrementClickCount(shortCode string) error
}

type memoryURLStore struct {
	mu     sync.RWMutex
	urls   map[string]string
	clicks map[string]int
}

type server struct {
	storage urlStorage
}

func newStorage() (urlStorage, func() error, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return newMemoryURLStore(), func() error { return nil }, nil
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

func newMemoryURLStore() *memoryURLStore {
	return &memoryURLStore{
		urls:   make(map[string]string),
		clicks: make(map[string]int),
	}
}

func (s *memoryURLStore) Save(shortCode, targetURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[shortCode]; exists {
		return errShortCodeExists
	}

	s.urls[shortCode] = targetURL
	s.clicks[shortCode] = 0
	return nil
}

func (s *memoryURLStore) Lookup(shortCode string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetURL, ok := s.urls[shortCode]
	if !ok {
		return "", errShortCodeNotFound
	}

	return targetURL, nil
}

func (s *memoryURLStore) IncrementClickCount(shortCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[shortCode]; !exists {
		return errShortCodeNotFound
	}

	s.clicks[shortCode]++
	return nil
}

func createShortCode(storage urlStorage, targetURL string) (string, error) {
	for {
		shortCode, err := generateShortCode()
		if err != nil {
			return "", err
		}

		if err := storage.Save(shortCode, targetURL); err == nil {
			return shortCode, nil
		} else if !errors.Is(err, errShortCodeExists) {
			return "", err
		}
	}
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

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *server) handleShorten(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var req shortenRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "request body must contain a single JSON object", http.StatusBadRequest)
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	req.URL, err = normalizeTargetURL(req.URL)
	if err != nil {
		http.Error(w, "url must be a valid http or https URL", http.StatusBadRequest)
		return
	}

	shortCode, err := createShortCode(s.storage, req.URL)
	if err != nil {
		http.Error(w, "failed to generate short code", http.StatusInternalServerError)
		return
	}

	resp := shortenResponse{ShortCode: shortCode}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
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

	srv := &server{storage: storage}

	http.HandleFunc("/", srv.handleRoot)
	http.HandleFunc("/shorten", srv.handleShorten)

	addr := ":" + envOrDefault("PORT", "8081")
	if _, ok := storage.(*postgresURLStore); ok {
		log.Printf("using postgres storage\n")
	} else {
		log.Printf("using in-memory storage\n")
	}
	log.Printf("server listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
