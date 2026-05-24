package crypto

import (
	"errors"
	"strings"
	"testing"
)

func TestSeal_Roundtrip(t *testing.T) {
	key := DeriveKey([]byte("any-secret-from-env"))
	cases := []string{
		"hunter2",
		"",
		"contraseña con tildes y un \"quote\"",
		strings.Repeat("x", 1024),
	}
	for _, plain := range cases {
		ct, err := Seal(plain, key)
		if err != nil {
			t.Fatalf("seal %q: %v", plain, err)
		}
		got, err := Open(ct, key)
		if err != nil {
			t.Fatalf("open %q: %v", plain, err)
		}
		if got != plain {
			t.Errorf("roundtrip mismatch: got %q, want %q", got, plain)
		}
	}
}

func TestSeal_DifferentNoncePerCall(t *testing.T) {
	key := DeriveKey([]byte("secret"))
	a, _ := Seal("hello", key)
	b, _ := Seal("hello", key)
	if a == b {
		t.Errorf("two seals of the same plaintext produced identical output (nonce reuse?): %q", a)
	}
}

func TestOpen_WrongKey(t *testing.T) {
	k1 := DeriveKey([]byte("one"))
	k2 := DeriveKey([]byte("two"))
	ct, _ := Seal("payload", k1)
	if _, err := Open(ct, k2); err == nil {
		t.Errorf("open with wrong key should fail, got nil")
	}
}

func TestOpen_BadFormat(t *testing.T) {
	key := DeriveKey([]byte("secret"))
	if _, err := Open("not-a-token", key); !errors.Is(err, ErrBadFormat) {
		t.Errorf("got %v, want ErrBadFormat", err)
	}
	if _, err := Open("v1:not-base64!!!", key); err == nil {
		t.Errorf("expected base64 decode error, got nil")
	}
}
