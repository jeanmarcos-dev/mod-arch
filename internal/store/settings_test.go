package store

import (
	"errors"
	"testing"
)

func TestSettings_UpsertAndRead(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	if _, err := s.GetSetting(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing err = %v, want ErrNotFound", err)
	}

	if err := s.SetSetting(ctx, "access_mode", "open", "admin", 100); err != nil {
		t.Fatalf("set #1: %v", err)
	}
	got, err := s.GetSetting(ctx, "access_mode")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "open" {
		t.Errorf("value = %q", got)
	}

	if err := s.SetSetting(ctx, "access_mode", "closed", "admin", 200); err != nil {
		t.Fatalf("set #2: %v", err)
	}
	got, _ = s.GetSetting(ctx, "access_mode")
	if got != "closed" {
		t.Errorf("after upsert value = %q", got)
	}
}

func TestSettings_List(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	_ = s.SetSetting(ctx, "a", "1", "", 0)
	_ = s.SetSetting(ctx, "b", "2", "admin", 0)

	all, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 || all["a"] != "1" || all["b"] != "2" {
		t.Errorf("list = %+v", all)
	}
}

func TestSettings_Delete(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	_ = s.SetSetting(ctx, "k", "v", "", 0)
	if err := s.DeleteSetting(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetSetting(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete err = %v", err)
	}
}
