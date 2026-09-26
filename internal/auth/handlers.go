package auth

import (
	"encoding/json"
	"net/http"
	"time"
)

const CookieName = "session"

type Handler struct {
	users    Users
	sessions *SessionStore
	secure   bool
}

func NewHandler(users Users, sessions *SessionStore, secureCookies bool) *Handler {
	return &Handler{users: users, sessions: sessions, secure: secureCookies}
}

// RegisterRoutes wires the login/logout/me endpoints. These are
// intentionally left unauthenticated on the mux; login and logout couldn't
// work otherwise, and me is how a caller finds out it isn't authenticated.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.handleLogout)
	mux.HandleFunc("GET /api/auth/me", h.handleMe)
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

	if !h.users.Verify(req.Username, req.Password) {
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

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	username, ok := h.sessionUsername(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": username})
}

func (h *Handler) sessionUsername(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	return h.sessions.Validate(r.Context(), cookie.Value)
}

// RequireAuth wraps a handler so it 401s unless a valid session cookie is present.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.sessionUsername(r); !ok {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
