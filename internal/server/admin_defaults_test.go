package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateUIDefaults_Valid(t *testing.T) {
	d, ok, _ := validateUIDefaults(`{"narratorsEnabled":false,"mode":"compact","theme":"green","density":"compact","speed":8,"narratorMinTickMs":300}`)
	if !ok {
		t.Fatalf("expected ok")
	}
	if d.NarratorsEnabled || d.Mode != "compact" || d.Theme != "green" || d.Density != "compact" || d.Speed != 8 || d.NarratorMinTickMs != 300 {
		t.Errorf("parsed wrong: %+v", d)
	}
}

func TestValidateUIDefaults_PartialFillsDefaults(t *testing.T) {
	d, ok, _ := validateUIDefaults(`{"theme":"dark"}`)
	if !ok {
		t.Fatalf("expected ok")
	}
	base := defaultUIDefaults()
	if d.Theme != "dark" {
		t.Errorf("theme=%q, want dark", d.Theme)
	}
	if d.Mode != base.Mode || d.Speed != base.Speed {
		t.Errorf("missing fields not filled from defaults: %+v", d)
	}
}

func TestValidateUIDefaults_InvalidEnum(t *testing.T) {
	_, ok, offender := validateUIDefaults(`{"mode":"turbo"}`)
	if ok || offender != "turbo" {
		t.Errorf("ok=%v offender=%q, want false/turbo", ok, offender)
	}
}

func TestValidateUIDefaults_SpeedOutOfRange(t *testing.T) {
	if _, ok, _ := validateUIDefaults(`{"speed":999}`); ok {
		t.Errorf("speed 999 must be rejected")
	}
	if _, ok, _ := validateUIDefaults(`{"speed":0}`); ok {
		t.Errorf("speed 0 must be rejected")
	}
}

func TestApplyUIDefaults_PersistsAndReads(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, msg := s.applySetting(r, st, SettingUIDefaults, `{"narratorsEnabled":false,"mode":"compact"}`); status != http.StatusOK {
		t.Fatalf("status=%d msg=%q", status, msg)
	}
	got := readUIDefaults(r.Context(), st)
	if got.NarratorsEnabled || got.Mode != "compact" {
		t.Errorf("readUIDefaults=%+v", got)
	}
}

func TestApplyUIDefaults_InvalidRejected(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDefaults, `{"theme":"chartreuse"}`); status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestReadUIDefaults_FallbackWhenUnset(t *testing.T) {
	_, st := newUITestServer(t)
	got := readUIDefaults(t.Context(), st)
	if got != defaultUIDefaults() {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestApplyScheduleTZ_ValidAndInvalid(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIScheduleTZ, "America/Caracas"); status != http.StatusOK {
		t.Fatalf("valid tz rejected")
	}
	if got := readScheduleTZ(r.Context(), st); got != "America/Caracas" {
		t.Errorf("tz=%q", got)
	}
	if status, _ := s.applySetting(r, st, SettingUIScheduleTZ, "Mars/Olympus"); status != http.StatusBadRequest {
		t.Errorf("invalid tz status=%d, want 400", status)
	}
}
