package server

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"basic", "Ada", "Ada", nil},
		{"trim", "  Ada  ", "Ada", nil},
		{"accents kept", "José", "José", nil},
		{"empty rejected", "", "", ErrNameRequired},
		{"only spaces rejected", "   ", "", ErrNameRequired},
		{"max length ok", strings.Repeat("a", 32), strings.Repeat("a", 32), nil},
		{"too long", strings.Repeat("a", 33), "", ErrNameTooLong},
		{"newline rejected", "Ada\nLovelace", "", ErrNameInvalid},
		{"tab rejected", "Ada\tLovelace", "", ErrNameInvalid},
		{"null byte rejected", "Ada\x00", "", ErrNameInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanName(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanEmail(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"basic", "Ada@example.com", "ada@example.com", nil},
		{"trim", "  ada@example.com  ", "ada@example.com", nil},
		{"empty allowed", "", "", nil},
		{"missing dot", "ada@example", "", ErrEmailFormat},
		{"missing at", "ada.example.com", "", ErrEmailFormat},
		{"spaces inside", "a a@example.com", "", ErrEmailFormat},
		{"too long", strings.Repeat("a", 250) + "@b.co", "", ErrEmailTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanEmail(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanStudentID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"basic", "ABC123", "ABC123", nil},
		{"trim", "  ABC-123.x  ", "ABC-123.x", nil},
		{"empty allowed", "", "", nil},
		{"space inside", "ABC 123", "", ErrStudentIDFormat},
		{"unicode rejected", "Adá123", "", ErrStudentIDFormat},
		{"too long", strings.Repeat("A", 33), "", ErrStudentIDFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanStudentID(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanLabel(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"basic", "Pizarra de Ada", "Pizarra de Ada", nil},
		{"empty allowed", "", "", nil},
		{"trim", "  Ada  ", "Ada", nil},
		{"too long", strings.Repeat("a", 33), "", ErrLabelTooLong},
		{"control char", "Ada\x01", "", ErrLabelInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanLabel(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
