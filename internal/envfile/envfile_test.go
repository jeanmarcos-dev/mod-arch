package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_BasicAssignments(t *testing.T) {
	path := writeTempFile(t, `
# leading comment
FOO=bar
EMPTY=
QUOTED="hello world"
SINGLE='one two'
   PADDED = value
`)
	t.Setenv("FOO", "")
	t.Setenv("EMPTY", "")
	t.Setenv("QUOTED", "")
	t.Setenv("SINGLE", "")
	t.Setenv("PADDED", "")
	_ = os.Unsetenv("FOO")
	_ = os.Unsetenv("EMPTY")
	_ = os.Unsetenv("QUOTED")
	_ = os.Unsetenv("SINGLE")
	_ = os.Unsetenv("PADDED")

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := map[string]string{
		"FOO":    "bar",
		"EMPTY":  "",
		"QUOTED": "hello world",
		"SINGLE": "one two",
		"PADDED": "value",
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoad_DoesNotOverrideExistingEnv(t *testing.T) {
	path := writeTempFile(t, "FOO=from_file\n")
	t.Setenv("FOO", "from_env")
	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("FOO"); got != "from_env" {
		t.Errorf("FOO = %q, want from_env (real env should win)", got)
	}
}

func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "no-such.env")); err != nil {
		t.Errorf("missing file should be silent, got %v", err)
	}
}

func TestLoad_IgnoresMalformedLines(t *testing.T) {
	path := writeTempFile(t, "junk-without-equals\nGOOD=yes\n")
	_ = os.Unsetenv("GOOD")
	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("GOOD"); got != "yes" {
		t.Errorf("GOOD = %q", got)
	}
}

func writeTempFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}
