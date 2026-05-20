// Package envfile loads KEY=VALUE pairs from a simple .env file into the
// process environment without overriding variables already set by the OS.
//
// Supported syntax:
//   - One assignment per line: KEY=VALUE
//   - Blank lines and `#` comments are ignored.
//   - Surrounding double or single quotes on VALUE are stripped.
//   - No interpolation, no multi-line values, no `export` keyword.
package envfile

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Load reads the file at path and exports its entries via os.Setenv.
// Returns nil when the file does not exist so callers can treat .env as
// optional with no special-casing.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("envfile: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		if key == "" {
			return fmt.Errorf("envfile: %q:%d: missing key", path, lineNo)
		}
		// Real environment wins so deployment-level config keeps overriding the file.
		if _, present := os.LookupEnv(key); present {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("envfile: setenv %q: %w", key, err)
		}
	}
	return sc.Err()
}

// parseLine extracts (key, value) from one .env line. Returns ok=false for
// blank lines and comments.
func parseLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	rawKey, rawValue, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key := strings.TrimSpace(rawKey)
	value := strings.TrimSpace(rawValue)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
