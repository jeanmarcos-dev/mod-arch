package server

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field length limits in runes. Email follows RFC 5321; the rest are picked
// to keep UI tidy without truncating real-world inputs.
const (
	MaxNameRunes      = 32
	MaxEmailRunes     = 254
	MaxStudentIDRunes = 32
	MaxLabelRunes     = 32
)

// Validation errors are surfaced to the caller as-is (Spanish copy), so the
// HTTP handlers can pass them straight through.
var (
	ErrNameRequired    = errors.New("el nombre es obligatorio")
	ErrNameTooLong     = errors.New("el nombre no puede exceder 32 caracteres")
	ErrNameInvalid     = errors.New("el nombre contiene caracteres no permitidos")
	ErrEmailFormat     = errors.New("el correo no tiene un formato válido")
	ErrEmailTooLong    = errors.New("el correo es demasiado largo")
	ErrStudentIDFormat = errors.New("el ID de estudiante solo admite letras, números, punto y guión")
	ErrLabelTooLong    = errors.New("el alias no puede exceder 32 caracteres")
	ErrLabelInvalid    = errors.New("el alias contiene caracteres no permitidos")
)

var (
	validEmailRE     = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	validStudentIDRE = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
)

// cleanName trims, validates UTF-8, rejects control chars and enforces the
// length limit. Empty input is rejected — name is the only mandatory field.
func cleanName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrNameRequired
	}
	if !utf8.ValidString(s) {
		return "", ErrNameInvalid
	}
	if hasControlRunes(s) {
		return "", ErrNameInvalid
	}
	if utf8.RuneCountInString(s) > MaxNameRunes {
		return "", ErrNameTooLong
	}
	return s, nil
}

// cleanEmail returns an empty string when input is blank (email is optional).
// When present, it is lowercased and validated against the format regex.
func cleanEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", nil
	}
	if utf8.RuneCountInString(s) > MaxEmailRunes {
		return "", ErrEmailTooLong
	}
	if !validEmailRE.MatchString(s) {
		return "", ErrEmailFormat
	}
	return s, nil
}

// cleanStudentID returns "" for blank input. Otherwise enforces the strict
// charset to avoid surprises in joins/queries.
func cleanStudentID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !validStudentIDRE.MatchString(s) {
		return "", ErrStudentIDFormat
	}
	return s, nil
}

// cleanLabel sanitizes an alias-style display name. Empty input is allowed
// (callers treat it as "no override"). Control chars are rejected so the
// label is safe to render anywhere without escaping surprises.
func cleanLabel(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !utf8.ValidString(s) {
		return "", ErrLabelInvalid
	}
	if hasControlRunes(s) {
		return "", ErrLabelInvalid
	}
	if utf8.RuneCountInString(s) > MaxLabelRunes {
		return "", ErrLabelTooLong
	}
	return s, nil
}

// hasControlRunes reports whether s contains any C0/C1 control character.
// Tab, line-feed and carriage-return are also rejected: identity fields
// should be single-line.
func hasControlRunes(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
