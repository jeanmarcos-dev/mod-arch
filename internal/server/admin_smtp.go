package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/crypto"
	"github.com/jeanmarcos-dev/mod-arch/internal/mail"
)

// Settings keys for the SMTP block. Kept together so the admin handler and
// the magic-link sender read from the same source of truth.
const (
	smtpKeyHost          = "smtp_host"
	smtpKeyPort          = "smtp_port"
	smtpKeyUser          = "smtp_user"
	smtpKeyPasswordEnc   = "smtp_password_enc"
	smtpKeyFromName      = "smtp_from_name"
	smtpKeyFromEmail     = "smtp_from_email"
	smtpKeyReplyTo       = "smtp_reply_to"
	smtpKeySubjectPrefix = "smtp_subject_prefix"
	smtpKeyBCC           = "smtp_bcc"
	smtpKeyTLSMode       = "smtp_tls_mode"
	smtpKeyEnabled       = "smtp_enabled"
)

// smtpView is what the SPA reads. We never echo the password back — the
// boolean `password_set` lets the form show "(sin cambios)" instead.
type smtpView struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	PasswordSet   bool   `json:"password_set"`
	FromName      string `json:"from_name"`
	FromEmail     string `json:"from_email"`
	ReplyTo       string `json:"reply_to"`
	SubjectPrefix string `json:"subject_prefix"`
	BCC           string `json:"bcc"`
	TLSMode       string `json:"tls_mode"`
	Enabled       bool   `json:"enabled"`
}

// smtpInput mirrors the view but treats the password as a pointer so the SPA
// can distinguish "do not touch" (omitted) from "clear it" (explicit "").
type smtpInput struct {
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	User          string  `json:"user"`
	Password      *string `json:"password,omitempty"`
	FromName      string  `json:"from_name"`
	FromEmail     string  `json:"from_email"`
	ReplyTo       string  `json:"reply_to"`
	SubjectPrefix string  `json:"subject_prefix"`
	BCC           string  `json:"bcc"`
	TLSMode       string  `json:"tls_mode"`
	Enabled       bool    `json:"enabled"`
}

// loadSMTPView reads all SMTP keys and returns the JSON-friendly view. Used
// by the GET handler and by handleAdminMe when we eventually expose
// "smtp_configured" to the SPA badge.
func (s *Server) loadSMTPView(ctx context.Context) (smtpView, error) {
	st := s.manager.Store()
	if st == nil {
		return smtpView{}, fmt.Errorf("store unavailable")
	}
	v := smtpView{TLSMode: string(mail.TLSStartTLS), Port: 587}
	if x, _ := st.GetSetting(ctx, smtpKeyHost); x != "" {
		v.Host = x
	}
	if x, _ := st.GetSetting(ctx, smtpKeyPort); x != "" {
		if n, err := strconv.Atoi(x); err == nil {
			v.Port = n
		}
	}
	v.User, _ = st.GetSetting(ctx, smtpKeyUser)
	if enc, _ := st.GetSetting(ctx, smtpKeyPasswordEnc); enc != "" {
		v.PasswordSet = true
	}
	v.FromName, _ = st.GetSetting(ctx, smtpKeyFromName)
	v.FromEmail, _ = st.GetSetting(ctx, smtpKeyFromEmail)
	v.ReplyTo, _ = st.GetSetting(ctx, smtpKeyReplyTo)
	v.SubjectPrefix, _ = st.GetSetting(ctx, smtpKeySubjectPrefix)
	v.BCC, _ = st.GetSetting(ctx, smtpKeyBCC)
	if m, _ := st.GetSetting(ctx, smtpKeyTLSMode); m != "" {
		v.TLSMode = m
	}
	if e, _ := st.GetSetting(ctx, smtpKeyEnabled); e == "true" {
		v.Enabled = true
	}
	return v, nil
}

// loadSMTPSender builds a mail.Sender ready to dispatch, decrypting the
// stored password. Returns nil sender when SMTP is disabled or incompletely
// configured so callers can decide to skip silently.
func (s *Server) loadSMTPSender(ctx context.Context) (*mail.Sender, error) {
	v, err := s.loadSMTPView(ctx)
	if err != nil {
		return nil, err
	}
	if !v.Enabled || v.Host == "" || v.FromEmail == "" {
		return nil, nil
	}
	st := s.manager.Store()
	encPass, _ := st.GetSetting(ctx, smtpKeyPasswordEnc)
	plainPass := ""
	if encPass != "" {
		key := crypto.DeriveKey(s.admin.secret)
		p, err := crypto.Open(encPass, key)
		if err != nil {
			return nil, fmt.Errorf("decrypt smtp password: %w", err)
		}
		plainPass = p
	}
	return mail.New(mail.Config{
		Host:          v.Host,
		Port:          v.Port,
		User:          v.User,
		Password:      plainPass,
		FromName:      v.FromName,
		FromEmail:     v.FromEmail,
		ReplyTo:       v.ReplyTo,
		SubjectPrefix: v.SubjectPrefix,
		BCC:           v.BCC,
		TLSMode:       mail.TLSMode(v.TLSMode),
	}), nil
}

// validateSMTPInput keeps the input rules in one place so the GET and PUT
// agree on what "valid" means.
func validateSMTPInput(in smtpInput) error {
	if in.Enabled {
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("el host es obligatorio cuando se habilita el envío")
		}
		if in.Port <= 0 || in.Port > 65535 {
			return fmt.Errorf("el puerto debe estar entre 1 y 65535")
		}
		if strings.TrimSpace(in.FromEmail) == "" {
			return fmt.Errorf("la dirección del remitente es obligatoria")
		}
	}
	for label, addr := range map[string]string{
		"remitente":  in.FromEmail,
		"reply-to":   in.ReplyTo,
		"copia BCC":  in.BCC,
	} {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if !validEmailRE.MatchString(strings.ToLower(addr)) {
			return fmt.Errorf("la dirección de %s no tiene formato válido", label)
		}
	}
	if in.FromName != "" {
		if hasControlRunes(in.FromName) {
			return fmt.Errorf("el nombre del remitente contiene caracteres no permitidos")
		}
		if len(in.FromName) > 100 {
			return fmt.Errorf("el nombre del remitente excede 100 caracteres")
		}
	}
	if in.SubjectPrefix != "" {
		if hasControlRunes(in.SubjectPrefix) {
			return fmt.Errorf("el prefijo del asunto contiene caracteres no permitidos")
		}
		if len(in.SubjectPrefix) > 64 {
			return fmt.Errorf("el prefijo del asunto excede 64 caracteres")
		}
	}
	switch in.TLSMode {
	case string(mail.TLSStartTLS), string(mail.TLSImplicit), string(mail.TLSNone), "":
	default:
		return fmt.Errorf("modo TLS desconocido")
	}
	return nil
}

// handleAdminGetSMTP returns the persisted SMTP config (password redacted).
func (s *Server) handleAdminGetSMTP(w http.ResponseWriter, r *http.Request) {
	v, err := s.loadSMTPView(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// handleAdminSetSMTP upserts every SMTP key, encrypting the password when
// present. The audit log records which keys changed so a leaked secret can
// be traced back to the moment of update without exposing its value.
func (s *Server) handleAdminSetSMTP(w http.ResponseWriter, r *http.Request) {
	var body smtpInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	if err := validateSMTPInput(body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}

	actor := s.adminActor(r)
	now := time.Now().Unix()
	changed := make([]string, 0, 11)
	saveField := func(key, value string, label string) error {
		current, _ := st.GetSetting(r.Context(), key)
		if current == value {
			return nil
		}
		if err := st.SetSetting(r.Context(), key, value, actor, now); err != nil {
			return err
		}
		changed = append(changed, label)
		return nil
	}

	if err := saveField(smtpKeyHost, strings.TrimSpace(body.Host), "host"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyPort, strconv.Itoa(body.Port), "port"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyUser, strings.TrimSpace(body.User), "user"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyFromName, strings.TrimSpace(body.FromName), "from_name"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyFromEmail, strings.ToLower(strings.TrimSpace(body.FromEmail)), "from_email"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyReplyTo, strings.ToLower(strings.TrimSpace(body.ReplyTo)), "reply_to"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeySubjectPrefix, strings.TrimSpace(body.SubjectPrefix), "subject_prefix"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	if err := saveField(smtpKeyBCC, strings.ToLower(strings.TrimSpace(body.BCC)), "bcc"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	mode := body.TLSMode
	if mode == "" {
		mode = string(mail.TLSStartTLS)
	}
	if err := saveField(smtpKeyTLSMode, mode, "tls_mode"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}
	enabled := "false"
	if body.Enabled {
		enabled = "true"
	}
	if err := saveField(smtpKeyEnabled, enabled, "enabled"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
		return
	}

	if body.Password != nil {
		if *body.Password == "" {
			if err := st.DeleteSetting(r.Context(), smtpKeyPasswordEnc); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo borrar la contraseña"})
				return
			}
			changed = append(changed, "password_cleared")
		} else {
			key := crypto.DeriveKey(s.admin.secret)
			ct, err := crypto.Seal(*body.Password, key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo cifrar la contraseña"})
				return
			}
			if err := st.SetSetting(r.Context(), smtpKeyPasswordEnc, ct, actor, now); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar"})
				return
			}
			changed = append(changed, "password")
		}
	}

	meta, _ := json.Marshal(map[string]any{"changed": changed})
	s.audit(r, "smtp.update", "", string(meta))

	out, _ := s.loadSMTPView(r.Context())
	writeJSON(w, http.StatusOK, out)
}

// handleAdminTestSMTP sends a canned message to verify the live config. Body
// is {to: "addr"}. The error message includes the underlying SMTP failure
// so the admin can debug auth/host/port issues straight from the form.
func (s *Server) handleAdminTestSMTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	to := strings.ToLower(strings.TrimSpace(body.To))
	if !validEmailRE.MatchString(to) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "el destinatario no tiene formato válido"})
		return
	}

	sender, err := s.loadSMTPSender(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if sender == nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"error": "la SMTP no está configurada o está deshabilitada"})
		return
	}

	subject := "MODOS-CPU · prueba de SMTP"
	textBody := "Si recibió este mensaje, la configuración SMTP del panel MODOS-CPU funciona correctamente.\n"
	htmlBody := "<p>Si recibió este mensaje, la configuración SMTP del panel <strong>MODOS-CPU</strong> funciona correctamente.</p>"

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	auditMeta := map[string]any{"to": to, "ok": false}
	if err := sender.Send(ctx, to, subject, htmlBody, textBody); err != nil {
		auditMeta["error"] = err.Error()
		meta, _ := json.Marshal(auditMeta)
		s.audit(r, "smtp.test", to, string(meta))
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	auditMeta["ok"] = true
	meta, _ := json.Marshal(auditMeta)
	s.audit(r, "smtp.test", to, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sendMagicLinkEmail wraps loadSMTPSender + Send with the canonical magic-link
// copy. Returns errSMTPNotConfigured when no mail backend is set up so callers
// can degrade gracefully, and any other error when SMTP is configured but the
// delivery failed (treated as a hard failure by handleAuthLogin).
func (s *Server) sendMagicLinkEmail(ctx context.Context, to, url string) error {
	if sendMagicLinkEmailHook != nil {
		return sendMagicLinkEmailHook(ctx, to, url)
	}
	sender, err := s.loadSMTPSender(ctx)
	if err != nil {
		return err
	}
	if sender == nil {
		return errSMTPNotConfigured
	}
	subject := "Su acceso a MODOS-CPU"
	textBody := "Para entrar a su sesión, abra el siguiente enlace (vence en 15 minutos):\n\n" + url + "\n"
	htmlBody := "<p>Para entrar a su sesión, abra el siguiente enlace (vence en 15 minutos):</p>" +
		"<p><a href=\"" + url + "\">" + url + "</a></p>"
	return sender.Send(ctx, to, subject, htmlBody, textBody)
}

// sendMagicLinkEmailHook lets tests bypass real SMTP. Production never sets
// it; the auth_test suite swaps it in to exercise the success path without
// standing up a real mail server.
var sendMagicLinkEmailHook func(ctx context.Context, to, url string) error

var errSMTPNotConfigured = errors.New("smtp not configured")
