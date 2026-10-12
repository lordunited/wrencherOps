// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Handler serves who the signed-in user is and what they can do, their
// own password, and — for admins — users, groups and access.
type Handler struct {
	store    *Store
	resolver *Resolver
}

func NewHandler(store *Store, resolver *Resolver) *Handler {
	return &Handler{store: store, resolver: resolver}
}

// AdminRoutes are the admin API's routes; each needs an admin.
var AdminRoutes = map[string]func(*Handler, http.ResponseWriter, *http.Request){
	"GET /api/admin/tabs":                      (*Handler).handleTabs,
	"GET /api/admin/users":                     (*Handler).handleUsers,
	"POST /api/admin/users":                    (*Handler).handleCreateUser,
	"GET /api/admin/users/{username}":          (*Handler).handleUser,
	"PUT /api/admin/users/{username}":          (*Handler).handleUpdateUser,
	"DELETE /api/admin/users/{username}":       (*Handler).handleDeleteUser,
	"PUT /api/admin/users/{username}/password": (*Handler).handleResetPassword,
	"PUT /api/admin/users/{username}/grants":   (*Handler).handleUserGrants,
	"GET /api/admin/groups":                    (*Handler).handleGroups,
	"POST /api/admin/groups":                   (*Handler).handleCreateGroup,
	"GET /api/admin/groups/{id}":               (*Handler).handleGroup,
	"PUT /api/admin/groups/{id}":               (*Handler).handleUpdateGroup,
	"DELETE /api/admin/groups/{id}":            (*Handler).handleDeleteGroup,
	"PUT /api/admin/groups/{id}/grants":        (*Handler).handleGroupGrants,
}

// PathPrefixes are the URL prefixes RegisterRoutes serves.
func (h *Handler) PathPrefixes() []string {
	return []string{"/api/auth/me", "/api/account/", "/api/admin/"}
}

func (h *Handler) RegisterRoutes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("GET /api/auth/me", h.handleMe)
	mux.HandleFunc("POST /api/account/password", h.handleOwnPassword)
	for pattern, f := range AdminRoutes {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) { f(h, w, r) })
	}
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// badRequest is an error that's the caller's to fix.
type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func bad(format string, a ...any) error { return badRequest{fmt.Sprintf(format, a...)} }

func writeErr(w http.ResponseWriter, err error) {
	var br badRequest
	switch {
	case errors.As(err, &br):
		http.Error(w, br.msg, http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrExists):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return bad("invalid request body")
	}
	return nil
}

// --- the signed-in user

type meResponse struct {
	Username string            `json:"username"`
	Role     string            `json:"role"`
	Builtin  bool              `json:"builtin"`
	Tabs     map[string]string `json:"tabs"`
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	a := FromContext(r.Context())
	writeJSON(w, meResponse{Username: a.Username, Role: a.Role, Builtin: a.Builtin, Tabs: a.TabLevels()}, nil)
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func (h *Handler) handleOwnPassword(w http.ResponseWriter, r *http.Request) {
	a := FromContext(r.Context())
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if a.Builtin {
		writeErr(w, bad("%s is a built-in account: its password is set in the panel's AUTH_USERS setting, not here", a.Username))
		return
	}
	if !h.resolver.Verify(r.Context(), a.Username, req.Current) {
		http.Error(w, "the current password is wrong", http.StatusForbidden)
		return
	}
	if err := CheckPassword(req.New, a.Username); err != nil {
		writeErr(w, bad("%s", err))
		return
	}
	hash, err := hashPassword(req.New)
	if err == nil {
		err = h.store.SetPasswordHash(r.Context(), a.Username, hash)
	}
	if err == nil {
		// Signed out everywhere else: whoever knew the old password.
		keep := ""
		if c, cerr := r.Cookie(CookieName); cerr == nil {
			keep = c.Value
		}
		err = h.store.EndSessions(r.Context(), a.Username, keep)
	}
	if err == nil {
		log.Printf("access: %s changed their password", a.Username)
	}
	writeJSON(w, map[string]bool{"ok": true}, err)
}

// --- admin: tabs and users

func (h *Handler) handleTabs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Tabs, nil)
}

// userView is a user for the admin pages: a database user, or a built-in
// one.
type userView struct {
	*User
	Grants    map[string]string `json:"grants"`
	Effective map[string]string `json:"effective"`
}

func (h *Handler) builtinUsers() []*User {
	var names []string
	for n := range h.resolver.builtin {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*User, 0, len(names))
	for _, n := range names {
		out = append(out, &User{Username: n, Role: RoleAdmin, Builtin: true, Groups: []string{}})
	}
	return out
}

func (h *Handler) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.Users(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, append(h.builtinUsers(), users...), nil)
}

func levelNames(m map[string]Level) map[string]string {
	out := map[string]string{}
	for k, l := range m {
		if l > None {
			out[k] = l.String()
		}
	}
	return out
}

func (h *Handler) userView(r *http.Request, username string) (*userView, error) {
	if h.resolver.IsBuiltin(username) {
		return &userView{User: &User{Username: username, Role: RoleAdmin, Builtin: true, Groups: []string{}},
			Grants: map[string]string{}, Effective: levelNames(Effective(RoleAdmin))}, nil
	}
	u, err := h.store.User(r.Context(), username)
	if err != nil {
		return nil, err
	}
	direct, err := h.store.Grants(r.Context(), SubjectUser, username)
	if err != nil {
		return nil, err
	}
	all := []map[string]Level{direct}
	for _, g := range u.Groups {
		gg, err := h.store.Grants(r.Context(), SubjectGroup, g)
		if err != nil {
			return nil, err
		}
		all = append(all, gg)
	}
	eff := Effective(u.Role, all...)
	if u.Disabled {
		eff = map[string]Level{}
	}
	return &userView{User: u, Grants: levelNames(direct), Effective: levelNames(eff)}, nil
}

func (h *Handler) handleUser(w http.ResponseWriter, r *http.Request) {
	v, err := h.userView(r, r.PathValue("username"))
	writeJSON(w, v, err)
}

type userRequest struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     string   `json:"role"`
	Disabled bool     `json:"disabled"`
	Groups   []string `json:"groups"`
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	switch {
	case !ValidUsername(req.Username):
		writeErr(w, bad("a username is 2 to 63 lowercase letters, digits, dots, dashes or underscores, starting with a letter or digit"))
		return
	case h.resolver.IsBuiltin(req.Username):
		writeErr(w, bad("%s is a built-in account already", req.Username))
		return
	case !ValidRole(req.Role):
		writeErr(w, bad("the role must be admin, writer or viewer"))
		return
	}
	if err := CheckPassword(req.Password, req.Username); err != nil {
		writeErr(w, bad("%s", err))
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.store.CreateUser(r.Context(), req.Username, hash, req.Role); err != nil {
		if errors.Is(err, ErrExists) {
			err = fmt.Errorf("%w: a user named %s", ErrExists, req.Username)
		}
		writeErr(w, err)
		return
	}
	if len(req.Groups) > 0 {
		if err := h.store.SetUserGroups(r.Context(), req.Username, req.Groups); err != nil {
			writeErr(w, err)
			return
		}
	}
	h.resolver.Forget()
	log.Printf("access: %s created user %s (%s)", FromContext(r.Context()).Username, req.Username, req.Role)
	v, err := h.userView(r, req.Username)
	writeJSON(w, v, err)
}

// editable refuses changes to built-in accounts and, for the ones that
// would lock an admin out, to the admin's own.
func (h *Handler) editable(r *http.Request, username string, selfToo bool) error {
	if h.resolver.IsBuiltin(username) {
		return bad("%s is a built-in admin, set in the panel's AUTH_USERS setting; it can't be changed here", username)
	}
	if !selfToo && FromContext(r.Context()).Username == username {
		return bad("you can't do this to your own account; ask another admin")
	}
	return nil
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	var req userRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.editable(r, name, true); err != nil {
		writeErr(w, err)
		return
	}
	if !ValidRole(req.Role) {
		writeErr(w, bad("the role must be admin, writer or viewer"))
		return
	}
	if FromContext(r.Context()).Username == name && (req.Role != RoleAdmin || req.Disabled) {
		writeErr(w, bad("you can't take away your own admin role or disable yourself; ask another admin"))
		return
	}
	err := h.store.UpdateUser(r.Context(), name, req.Role, req.Disabled)
	if err == nil && req.Groups != nil {
		err = h.store.SetUserGroups(r.Context(), name, req.Groups)
	}
	if err == nil && req.Disabled {
		err = h.store.EndSessions(r.Context(), name, "")
	}
	h.resolver.Forget()
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("access: %s updated user %s (role %s, disabled %v)", FromContext(r.Context()).Username, name, req.Role, req.Disabled)
	v, err := h.userView(r, name)
	writeJSON(w, v, err)
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	if err := h.editable(r, name, false); err != nil {
		writeErr(w, err)
		return
	}
	err := h.store.DeleteUser(r.Context(), name)
	h.resolver.Forget()
	if err == nil {
		log.Printf("access: %s deleted user %s", FromContext(r.Context()).Username, name)
	}
	writeJSON(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.editable(r, name, true); err != nil {
		writeErr(w, err)
		return
	}
	if err := CheckPassword(req.Password, name); err != nil {
		writeErr(w, bad("%s", err))
		return
	}
	hash, err := hashPassword(req.Password)
	if err == nil {
		err = h.store.SetPasswordHash(r.Context(), name, hash)
	}
	if err == nil {
		keep := ""
		if c, cerr := r.Cookie(CookieName); cerr == nil && FromContext(r.Context()).Username == name {
			keep = c.Value
		}
		err = h.store.EndSessions(r.Context(), name, keep)
	}
	if err == nil {
		log.Printf("access: %s reset the password of %s", FromContext(r.Context()).Username, name)
	}
	writeJSON(w, map[string]bool{"ok": true}, err)
}

// parseGrants checks a grants map from the browser: known tabs, levels a
// tab can have.
func parseGrants(in map[string]string) (map[string]Level, error) {
	out := map[string]Level{}
	for key, s := range in {
		t, ok := TabByKey(key)
		if !ok {
			return nil, bad("there's no tab %q", key)
		}
		l, ok := ParseLevel(s)
		if !ok {
			return nil, bad("%q isn't a level: none, view or write", s)
		}
		if t.AdminOnly && l > None {
			return nil, bad("%s › %s is only for admins: make the user an admin instead", t.Category, t.Label)
		}
		if t.ViewOnly && l > View {
			return nil, bad("%s › %s can only be viewed", t.Category, t.Label)
		}
		if l > None {
			out[key] = l
		}
	}
	return out, nil
}

func (h *Handler) handleUserGrants(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	var req struct {
		Grants map[string]string `json:"grants"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.editable(r, name, true); err != nil {
		writeErr(w, err)
		return
	}
	grants, err := parseGrants(req.Grants)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.store.User(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	err = h.store.SetGrants(r.Context(), SubjectUser, name, grants)
	h.resolver.Forget()
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("access: %s set %s's access (%d tabs)", FromContext(r.Context()).Username, name, len(grants))
	v, err := h.userView(r, name)
	writeJSON(w, v, err)
}

// --- admin: groups

type groupView struct {
	*Group
	Grants map[string]string `json:"grants"`
}

func (h *Handler) groupView(r *http.Request, id string) (*groupView, error) {
	g, err := h.store.Group(r.Context(), id)
	if err != nil {
		return nil, err
	}
	grants, err := h.store.Grants(r.Context(), SubjectGroup, id)
	if err != nil {
		return nil, err
	}
	return &groupView{Group: g, Grants: levelNames(grants)}, nil
}

func (h *Handler) handleGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.Groups(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]*groupView, 0, len(groups))
	for _, g := range groups {
		grants, err := h.store.Grants(r.Context(), SubjectGroup, g.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, &groupView{Group: g, Grants: levelNames(grants)})
	}
	writeJSON(w, out, nil)
}

func (h *Handler) handleGroup(w http.ResponseWriter, r *http.Request) {
	v, err := h.groupView(r, r.PathValue("id"))
	writeJSON(w, v, err)
}

type groupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
}

func (req *groupRequest) check() error {
	req.Name, req.Description = strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	if req.Name == "" || len(req.Name) > 80 {
		return bad("a group needs a name, up to 80 characters")
	}
	return nil
}

func (h *Handler) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := req.check(); err != nil {
		writeErr(w, err)
		return
	}
	g, err := h.store.CreateGroup(r.Context(), req.Name, req.Description)
	if errors.Is(err, ErrExists) {
		err = fmt.Errorf("%w: a group named %s", ErrExists, req.Name)
	}
	if err == nil && len(req.Members) > 0 {
		err = h.store.UpdateGroup(r.Context(), g.ID, req.Name, req.Description, req.Members)
	}
	h.resolver.Forget()
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("access: %s created group %s", FromContext(r.Context()).Username, req.Name)
	v, err := h.groupView(r, g.ID)
	writeJSON(w, v, err)
}

func (h *Handler) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req groupRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := req.check(); err != nil {
		writeErr(w, err)
		return
	}
	if req.Members == nil {
		req.Members = []string{}
	}
	err := h.store.UpdateGroup(r.Context(), id, req.Name, req.Description, req.Members)
	if errors.Is(err, ErrExists) {
		err = fmt.Errorf("%w: a group named %s", ErrExists, req.Name)
	}
	h.resolver.Forget()
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("access: %s updated group %s (%d members)", FromContext(r.Context()).Username, req.Name, len(req.Members))
	v, err := h.groupView(r, id)
	writeJSON(w, v, err)
}

func (h *Handler) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeleteGroup(r.Context(), r.PathValue("id"))
	h.resolver.Forget()
	if err == nil {
		log.Printf("access: %s deleted group %s", FromContext(r.Context()).Username, r.PathValue("id"))
	}
	writeJSON(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) handleGroupGrants(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Grants map[string]string `json:"grants"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	grants, err := parseGrants(req.Grants)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.store.Group(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	err = h.store.SetGrants(r.Context(), SubjectGroup, id, grants)
	h.resolver.Forget()
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("access: %s set group %s's access (%d tabs)", FromContext(r.Context()).Username, id, len(grants))
	v, err := h.groupView(r, id)
	writeJSON(w, v, err)
}
