// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const CookieName = "session"

// Verifier checks a username and password: the built-in accounts and the
// panel's own users (access.Resolver is one).
type Verifier interface {
	Verify(ctx context.Context, username, password string) bool
}

type Handler struct {
	verifier Verifier
	sessions *SessionStore
	secure   bool
}

func NewHandler(verifier Verifier, sessions *SessionStore, secureCookies bool) *Handler {
	return &Handler{verifier: verifier, sessions: sessions, secure: secureCookies}
}

// RegisterRoutes wires the login/logout endpoints. These are intentionally
// left unauthenticated on the mux; they couldn't work otherwise. Who is
// signed in (GET /api/auth/me) is served behind the access guard.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.handleLogout)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !h.verifier.Verify(r.Context(), req.Username, req.Password) {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	token, expiresAt, err := h.sessions.Create(r.Context(), req.Username)
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": req.Username})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		h.sessions.Delete(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
}
