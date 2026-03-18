package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessCookieName  = "access_token"
	refreshCookieName = "refresh_token"
)

var (
	errUserExists             = errors.New("user already exists")
	errUserNotFound           = errors.New("user not found")
	errInvalidRefreshToken    = errors.New("invalid refresh token")
	errForbidden              = errors.New("forbidden")
	errInvalidOrigin          = errors.New("invalid origin")
	errInvalidJSONContentType = errors.New("content type must be application/json")
	errInvalidJSONBody        = errors.New("invalid json body")
	errMultipleJSONObjects    = errors.New("request body must contain a single JSON object")
	errAuthenticationRequired = errors.New("authentication required")
	errInvalidAccessToken     = errors.New("invalid access token")
)

type userRecord struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ownedLinkResponse struct {
	ShortCode  string    `json:"short_code"`
	TargetURL  string    `json:"target_url"`
	ClickCount int       `json:"click_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type authManager struct {
	accessSecret  []byte
	issuer        string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	secureCookies bool
}

func newAuthManagerFromEnv() (*authManager, error) {
	accessSecret := strings.TrimSpace(os.Getenv("JWT_ACCESS_SECRET"))
	if accessSecret == "" {
		return nil, errors.New("JWT_ACCESS_SECRET is required")
	}

	return &authManager{
		accessSecret:  []byte(accessSecret),
		issuer:        envOrDefault("JWT_ISSUER", "url-shortener"),
		accessTTL:     15 * time.Minute,
		refreshTTL:    7 * 24 * time.Hour,
		secureCookies: strings.EqualFold(envOrDefault("APP_ENV", "development"), "production"),
	}, nil
}

func (a *authManager) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func (a *authManager) verifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func (a *authManager) createAccessToken(userID int64) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    a.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(a.accessTTL)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.accessSecret)
}

func (a *authManager) parseAccessToken(tokenString string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errInvalidAccessToken
		}
		return a.accessSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, errInvalidAccessToken
	}
	if claims.Issuer != a.issuer {
		return 0, errInvalidAccessToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errInvalidAccessToken
	}

	return userID, nil
}

func (a *authManager) generateRefreshToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (a *authManager) setAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secureCookies,
		MaxAge:   int(a.accessTTL.Seconds()),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secureCookies,
		MaxAge:   int(a.refreshTTL.Seconds()),
	})
}

func (a *authManager) clearAuthCookies(w http.ResponseWriter) {
	for _, cookieName := range []string{accessCookieName, refreshCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   a.secureCookies,
			MaxAge:   -1,
		})
	}
}

func decodeJSONBody(r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errInvalidJSONContentType
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errInvalidJSONBody
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errMultipleJSONObjects
	}

	return nil
}

func writeJSONBodyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errInvalidJSONContentType):
		http.Error(w, errInvalidJSONContentType.Error(), http.StatusUnsupportedMediaType)
	case errors.Is(err, errMultipleJSONObjects):
		http.Error(w, errMultipleJSONObjects.Error(), http.StatusBadRequest)
	default:
		http.Error(w, errInvalidJSONBody.Error(), http.StatusBadRequest)
	}
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func normalizeEmail(rawEmail string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	if email == "" {
		return "", errors.New("email is required")
	}

	parsedAddress, err := mail.ParseAddress(email)
	if err != nil {
		return "", errors.New("email must be valid")
	}

	return strings.ToLower(parsedAddress.Address), nil
}

func validatePassword(password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	return nil
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.Split(forwardedProto, ",")[0]
	} else if r.TLS != nil {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}

func ensureSameOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	if origin != requestOrigin(r) {
		return errInvalidOrigin
	}

	return nil
}

func userToResponse(user userRecord) userResponse {
	return userResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}
}

func (s *server) currentUser(r *http.Request) (userRecord, error) {
	cookie, err := r.Cookie(accessCookieName)
	if err != nil {
		return userRecord{}, errAuthenticationRequired
	}

	userID, err := s.auth.parseAccessToken(cookie.Value)
	if err != nil {
		return userRecord{}, errAuthenticationRequired
	}

	user, err := s.storage.GetUserByID(userID)
	if errors.Is(err, errUserNotFound) {
		return userRecord{}, errAuthenticationRequired
	}
	if err != nil {
		return userRecord{}, err
	}

	return user, nil
}

func (s *server) requireCurrentUser(w http.ResponseWriter, r *http.Request) (userRecord, bool) {
	user, err := s.currentUser(r)
	if errors.Is(err, errAuthenticationRequired) {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return userRecord{}, false
	}
	if err != nil {
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return userRecord{}, false
	}

	return user, true
}

func (s *server) issueAuthSession(w http.ResponseWriter, user userRecord) error {
	accessToken, err := s.auth.createAccessToken(user.ID)
	if err != nil {
		return err
	}

	refreshToken, refreshHash, err := s.auth.generateRefreshToken()
	if err != nil {
		return err
	}

	if err := s.storage.StoreRefreshToken(user.ID, refreshHash, time.Now().UTC().Add(s.auth.refreshTTL)); err != nil {
		return err
	}

	s.auth.setAuthCookies(w, accessToken, refreshToken)
	return nil
}

func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ensureSameOrigin(r); err != nil {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}

	var req authRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONBodyError(w, err)
		return
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	passwordHash, err := s.auth.hashPassword(req.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	user, err := s.storage.CreateUser(email, passwordHash)
	if errors.Is(err, errUserExists) {
		http.Error(w, "user already exists", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	if err := s.issueAuthSession(w, user); err != nil {
		http.Error(w, "failed to create auth session", http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, http.StatusCreated, userToResponse(user))
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ensureSameOrigin(r); err != nil {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}

	var req authRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONBodyError(w, err)
		return
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	user, err := s.storage.GetUserByEmail(email)
	if errors.Is(err, errUserNotFound) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	if err := s.auth.verifyPassword(req.Password, user.PasswordHash); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := s.issueAuthSession(w, user); err != nil {
		http.Error(w, "failed to create auth session", http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, http.StatusOK, userToResponse(user))
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ensureSameOrigin(r); err != nil {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}

	if cookie, err := r.Cookie(refreshCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		_ = s.storage.RevokeRefreshToken(hashToken(cookie.Value))
	}

	s.auth.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ensureSameOrigin(r); err != nil {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}

	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		s.auth.clearAuthCookies(w)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	newRefreshToken, newRefreshHash, err := s.auth.generateRefreshToken()
	if err != nil {
		http.Error(w, "failed to refresh session", http.StatusInternalServerError)
		return
	}

	userID, err := s.storage.RotateRefreshToken(
		hashToken(cookie.Value),
		newRefreshHash,
		time.Now().UTC().Add(s.auth.refreshTTL),
	)
	if errors.Is(err, errInvalidRefreshToken) {
		s.auth.clearAuthCookies(w)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "failed to refresh session", http.StatusInternalServerError)
		return
	}

	accessToken, err := s.auth.createAccessToken(userID)
	if err != nil {
		http.Error(w, "failed to refresh session", http.StatusInternalServerError)
		return
	}

	s.auth.setAuthCookies(w, accessToken, newRefreshToken)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := s.requireCurrentUser(w, r)
	if !ok {
		return
	}

	writeJSONResponse(w, http.StatusOK, userToResponse(user))
}

func (s *server) handleLinks(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := s.requireCurrentUser(w, r)
	if !ok {
		return
	}

	links, err := s.storage.ListOwnedLinks(user.ID)
	if err != nil {
		http.Error(w, "failed to load links", http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, http.StatusOK, links)
}
