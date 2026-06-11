package server

import "github.com/jeanmarcos/modos-cpu/internal/sessions"

// SessionInfoEvent is the wire format for session metadata.
type SessionInfoEvent struct {
	Type      string            `json:"type"`
	ID        string            `json:"id"`
	Mode      string            `json:"mode"`
	Role      string            `json:"role"`
	Label     string            `json:"label"`
	ViewToken string            `json:"view_token"`
	UserID    string            `json:"user_id,omitempty"`
	UserName  string            `json:"user_name,omitempty"`
	UserEmail string            `json:"user_email,omitempty"`
	Presence  sessions.Presence `json:"presence"`
}

// PresenceEvent is the wire format for a presence update.
type PresenceEvent struct {
	Type      string         `json:"type"`
	Section   string         `json:"section"`
	Cores     []int          `json:"cores"`
	Mode      string         `json:"mode,omitempty"`
	Narrators bool           `json:"narrators,omitempty"`
	Focus     sessions.Focus `json:"focus,omitempty"`
	At        int64          `json:"at,omitempty"`
}

