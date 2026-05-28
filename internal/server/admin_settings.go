package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

type settingsResponse struct {
	AccessMode             string   `json:"access_mode"`
	ViewerPasswordRequired bool     `json:"viewer_password_required"`
	ViewerPasswordSet      bool     `json:"viewer_password_set"`
	DisabledRoutes         []string `json:"disabled_routes"`
	DisabledPanels         []string `json:"disabled_panels"`
	DisabledDemos          []string `json:"disabled_demos"`
	DefaultRoute           string   `json:"default_route"`
}

// handleAdminGetSettings returns the access-related settings as a flat object
// the SPA can bind to. The bcrypt hash itself is never returned — only the
// "is one configured?" boolean.
func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	mode := s.accessMode(r.Context())

	required := false
	if v, err := st.GetSetting(r.Context(), SettingViewerPasswordRequired); err == nil && v == "true" {
		required = true
	}
	hash, _ := st.GetSetting(r.Context(), SettingViewerPasswordHash)
	writeJSON(w, http.StatusOK, settingsResponse{
		AccessMode:             mode,
		ViewerPasswordRequired: required && hash != "",
		ViewerPasswordSet:      hash != "",
		DisabledRoutes:         readStringArraySetting(r.Context(), st, SettingUIDisabledRoutes),
		DisabledPanels:         readStringArraySetting(r.Context(), st, SettingUIDisabledPanels),
		DisabledDemos:          readStringArraySetting(r.Context(), st, SettingUIDisabledDemos),
		DefaultRoute:           readDefaultRoute(r.Context(), st),
	})
}

// handleAdminSetSetting accepts {key, value} and applies one of the known
// settings. Unknown keys are rejected to keep the surface tight.
func (s *Server) handleAdminSetSetting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}

	if status, msg := s.applySetting(r, st, body.Key, body.Value); status != http.StatusOK {
		writeJSON(w, status, map[string]any{"error": msg})
		return
	}
	meta := body.Value
	if body.Key == "viewer_password" {
		meta = "updated"
		if body.Value == "" {
			meta = "cleared"
		}
	}
	metaJSON, _ := json.Marshal(map[string]any{"value": meta})
	s.audit(r, "settings.set", body.Key, string(metaJSON))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// applySetting validates and persists one settings change. Returns
// (200, "") on success; otherwise an HTTP status and an error message
// suitable for the client.
func (s *Server) applySetting(r *http.Request, st *store.Store, key, value string) (int, string) {
	now := time.Now().Unix()
	actor := s.adminActor(r)
	switch key {
	case SettingAccessMode:
		if value != AccessOpen && value != AccessInviteOnly && value != AccessClosed {
			return http.StatusBadRequest, "valor inválido para access_mode"
		}
		return persistSetting(r, st, key, value, actor, now)
	case SettingViewerPasswordRequired:
		if value != "true" && value != "false" {
			return http.StatusBadRequest, "valor inválido (true|false)"
		}
		return persistSetting(r, st, key, value, actor, now)
	case "viewer_password":
		return s.applyViewerPassword(r, st, value, actor, now)
	case SettingUIDisabledRoutes:
		return applyUIArraySetting(r, st, key, value, isValidRouteName, actor, now)
	case SettingUIDisabledPanels:
		return applyUIArraySetting(r, st, key, value, isValidPanelID, actor, now)
	case SettingUIDisabledDemos:
		return applyUIArraySetting(r, st, key, value, isValidDemoName, actor, now)
	case SettingUIDefaultRoute:
		return applyUIDefaultRoute(r, st, value, actor, now)
	default:
		return http.StatusBadRequest, "clave desconocida"
	}
}

// applyUIArraySetting validates a JSON array of strings against the given
// member validator and persists the normalized form. Duplicates are
// collapsed; unknown identifiers are rejected with a 400 so the SPA cannot
// poison the catalog with typos.
func applyUIArraySetting(r *http.Request, st *store.Store, key, value string, valid func(string) bool, actor string, now int64) (int, string) {
	arr, ok, offender := validateStringArrayAgainst(value, valid)
	if !ok {
		if offender != "" {
			return http.StatusBadRequest, "identificador desconocido: " + offender
		}
		return http.StatusBadRequest, "valor inválido: se esperaba un arreglo JSON de strings"
	}
	// If the disabled-routes set would swallow the configured default route,
	// reject the change up front instead of leaving the SPA without a landing
	// page for students. The admin can either drop the offending route from
	// the list or move the default first.
	if key == SettingUIDisabledRoutes {
		current := readDefaultRoute(r.Context(), st)
		if slices.Contains(arr, current) {
			return http.StatusConflict, "no puede deshabilitar la pantalla por defecto (" + current + "); cambie el default primero"
		}
	}
	normalized, err := json.Marshal(arr)
	if err != nil {
		return http.StatusInternalServerError, "no se pudo serializar"
	}
	return persistSetting(r, st, key, string(normalized), actor, now)
}

// applyUIDefaultRoute validates that the chosen route exists and is not in
// the currently-disabled set. Empty value resets the default.
func applyUIDefaultRoute(r *http.Request, st *store.Store, value, actor string, now int64) (int, string) {
	if value == "" {
		if err := st.DeleteSetting(r.Context(), SettingUIDefaultRoute); err != nil {
			return http.StatusInternalServerError, "no se pudo guardar"
		}
		return http.StatusOK, ""
	}
	if !isValidRouteName(value) {
		return http.StatusBadRequest, "ruta desconocida: " + value
	}
	disabled := readStringArraySetting(r.Context(), st, SettingUIDisabledRoutes)
	if slices.Contains(disabled, value) {
		return http.StatusConflict, "la ruta " + value + " está deshabilitada; habilítela antes de fijarla como default"
	}
	return persistSetting(r, st, SettingUIDefaultRoute, value, actor, now)
}

func persistSetting(r *http.Request, st *store.Store, key, value, actor string, now int64) (int, string) {
	if err := st.SetSetting(r.Context(), key, value, actor, now); err != nil {
		return http.StatusInternalServerError, "no se pudo guardar"
	}
	return http.StatusOK, ""
}

// applyViewerPassword bcrypts the plaintext value before storing it, or
// clears the hash AND the required flag when the value is empty.
func (s *Server) applyViewerPassword(r *http.Request, st *store.Store, value, actor string, now int64) (int, string) {
	if value == "" {
		_ = st.DeleteSetting(r.Context(), SettingViewerPasswordHash)
		_ = st.SetSetting(r.Context(), SettingViewerPasswordRequired, "false", actor, now)
		return http.StatusOK, ""
	}
	h, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	if err != nil {
		return http.StatusInternalServerError, "no se pudo hashear"
	}
	if err := st.SetSetting(r.Context(), SettingViewerPasswordHash, string(h), actor, now); err != nil {
		return http.StatusInternalServerError, "no se pudo guardar"
	}
	return http.StatusOK, ""
}

type userResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Email       string             `json:"email,omitempty"`
	StudentID   string             `json:"student_id,omitempty"`
	Role        string             `json:"role"`
	LastLoginAt int64              `json:"last_login_at"`
	Sessions    []userSessionBrief `json:"sessions"`
}

type userSessionBrief struct {
	ID         string `json:"id"`
	Mode       string `json:"mode"`
	Label      string `json:"label"`
	LastActive int64  `json:"last_active"`
}

// handleAdminListUsers joins every persisted user with their currently live
// sessions (in-memory). Users with no active session still appear with an
// empty sessions array. When the caller authenticated via the env admin
// cookie, a synthetic row representing the configured ADMIN_USER is prepended
// so the env admin can see itself in the dashboard. The row is read-only
// (id == EnvAdminUserID) — the SPA already disables every action for that id.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	users, err := st.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudieron listar usuarios"})
		return
	}
	sessionsByUser := make(map[string][]userSessionBrief)
	for _, sess := range s.manager.All() {
		uid := sess.UserID()
		if uid == "" {
			continue
		}
		sessionsByUser[uid] = append(sessionsByUser[uid], userSessionBrief{
			ID:         sess.ID,
			Mode:       sess.Mode.String(),
			Label:      sess.Label(),
			LastActive: sess.LastActiveAt().Unix(),
		})
	}
	out := make([]userResponse, 0, len(users)+1)
	if callerID, _ := s.userRoleForRequest(r); callerID == EnvAdminUserID {
		out = append(out, userResponse{
			ID:          EnvAdminUserID,
			Name:        s.admin.user,
			Role:        store.RoleAdmin,
			LastLoginAt: 0,
			Sessions:    []userSessionBrief{},
		})
	}
	for _, u := range users {
		sess := sessionsByUser[u.ID]
		if sess == nil {
			sess = []userSessionBrief{}
		}
		out = append(out, userResponse{
			ID:          u.ID,
			Name:        u.Name,
			Email:       u.Email,
			StudentID:   u.StudentID,
			Role:        u.Role,
			LastLoginAt: u.LastLoginAt,
			Sessions:    sess,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAdminKickUser kills every live session attached to the user. The
// users row is kept so audit history and dedupe by email/student_id survive.
func (s *Server) handleAdminKickUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	if _, err := st.GetUser(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
		return
	}
	killed := 0
	for _, sess := range s.manager.All() {
		if sess.UserID() == id {
			s.manager.Kill(sess.ID)
			killed++
		}
	}
	meta, _ := json.Marshal(map[string]any{"sessions_killed": killed})
	s.audit(r, "user.kick", id, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions_killed": killed})
}

type auditResponse struct {
	ID     int64  `json:"id"`
	TS     int64  `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Meta   string `json:"meta,omitempty"`
}

// handleAdminListAudit returns the most recent entries, newest first.
func (s *Server) handleAdminListAudit(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	rows, err := st.ListAudit(r.Context(), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer el audit log"})
		return
	}
	out := make([]auditResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, auditResponse{
			ID: e.ID, TS: e.TS, Actor: e.Actor,
			Action: e.Action, Target: e.Target, Meta: e.MetaJSON,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAdminCreateUser inserts a fresh user row from admin input. At least
// one identifier (email or student_id) is required so a future magic-link
// login can dedupe onto this row; without one the entry would be unreachable.
// last_login_at = 0 marks it as never-logged-in for the UI.
func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		StudentID string `json:"student_id"`
		Role      string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	name, err := cleanName(body.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	email, err := cleanEmail(body.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	sid, err := cleanStudentID(body.StudentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if email == "" && sid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "debe indicar correo o ID de estudiante"})
		return
	}
	if !store.ValidRole(body.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "role inválido"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	row := store.UserRow{
		ID:        newAuthID(),
		Name:      name,
		Email:     email,
		StudentID: sid,
		Role:      body.Role,
		CreatedAt: time.Now().Unix(),
		// LastLoginAt stays at 0 until the user actually logs in.
	}
	if err := st.CreateUser(r.Context(), row); err != nil {
		switch {
		case errors.Is(err, store.ErrEmailTaken):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "ya existe un usuario activo con ese correo"})
		case errors.Is(err, store.ErrStudentIDTaken):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "ya existe un usuario activo con ese ID de estudiante"})
		default:
			s.logger.Warn("admin: create user failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo crear el usuario"})
		}
		return
	}
	meta, _ := json.Marshal(map[string]any{"role": body.Role, "email": email, "student_id": sid})
	s.audit(r, "user.create", row.ID, string(meta))
	writeJSON(w, http.StatusCreated, userResponse{
		ID:          row.ID,
		Name:        row.Name,
		Email:       row.Email,
		StudentID:   row.StudentID,
		Role:        row.Role,
		LastLoginAt: row.LastLoginAt,
		Sessions:    []userSessionBrief{},
	})
}

// handleAdminUpdateUser patches the identity columns. Role changes still go
// through handleAdminSetUserRole so the two surfaces remain independently
// auditable. Body fields are pointers: missing/null = leave alone; explicit
// empty string = clear the column.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == EnvAdminUserID {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "el admin por env no se puede editar"})
		return
	}
	var body struct {
		Name      *string `json:"name"`
		Email     *string `json:"email"`
		StudentID *string `json:"student_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	args := store.UpdateUserFieldsArgs{}
	changed := make([]string, 0, 3)
	if body.Name != nil {
		name, err := cleanName(*body.Name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		args.Name = &name
		changed = append(changed, "name")
	}
	if body.Email != nil {
		email, err := cleanEmail(*body.Email)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		args.Email = &email
		changed = append(changed, "email")
	}
	if body.StudentID != nil {
		sid, err := cleanStudentID(*body.StudentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		args.StudentID = &sid
		changed = append(changed, "student_id")
	}
	if len(changed) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "nada que actualizar"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	if err := st.UpdateUserFields(r.Context(), id, args); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
		case errors.Is(err, store.ErrEmailTaken):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "ya existe un usuario activo con ese correo"})
		case errors.Is(err, store.ErrStudentIDTaken):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "ya existe un usuario activo con ese ID de estudiante"})
		default:
			s.logger.Warn("admin: update user failed", "id", id, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo actualizar"})
		}
		return
	}
	updated, err := st.GetUser(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer el usuario"})
		return
	}
	meta, _ := json.Marshal(map[string]any{"changed": changed})
	s.audit(r, "user.update", id, string(meta))
	writeJSON(w, http.StatusOK, userResponse{
		ID:          updated.ID,
		Name:        updated.Name,
		Email:       updated.Email,
		StudentID:   updated.StudentID,
		Role:        updated.Role,
		LastLoginAt: updated.LastLoginAt,
		Sessions:    []userSessionBrief{},
	})
}

// handleAdminDeleteUser kills the user's active sessions and marks the row as
// soft-deleted. Soft deletion preserves audit lineage (actors stay
// resolvable) while freeing email/student_id for reuse via the partial
// indexes. The env admin and the caller itself are off limits.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == EnvAdminUserID {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "el admin por env no se puede eliminar"})
		return
	}
	callerID, _ := s.userRoleForRequest(r)
	if callerID == id {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "no puede eliminarse a sí mismo"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	killed := 0
	for _, sess := range s.manager.All() {
		if sess.UserID() == id {
			s.manager.Kill(sess.ID)
			killed++
		}
	}
	if err := st.DeleteUser(r.Context(), id, time.Now().Unix()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
			return
		}
		s.logger.Warn("admin: delete user failed", "id", id, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo eliminar"})
		return
	}
	meta, _ := json.Marshal(map[string]any{"sessions_killed": killed})
	s.audit(r, "user.delete", id, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions_killed": killed})
}

// handleAdminSetUserRole promotes or demotes a user. The env admin id
// (EnvAdminUserID) is rejected so the configured ADMIN_USER stays as the
// irrevocable recovery account. Admins cannot demote themselves either —
// blocking that path avoids accidental lock-outs where the only admin
// downgrades to student.
func (s *Server) handleAdminSetUserRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == EnvAdminUserID {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "el admin por env es irrevocable"})
		return
	}
	callerID, _ := s.userRoleForRequest(r)
	if callerID == id {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "no puede cambiar su propio rol"})
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	if !store.ValidRole(body.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "role inválido"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	prev, err := st.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer el usuario"})
		return
	}
	if err := st.UpdateUserRole(r.Context(), id, body.Role); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo actualizar"})
		return
	}
	meta, _ := json.Marshal(map[string]any{"from": prev.Role, "to": body.Role})
	s.audit(r, "user.role.set", id, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": body.Role})
}
