package staticui

import (
	"testing"
	"testing/fstest"
)

func TestFilesFromRequiresBuiltFrontend(t *testing.T) {
	if _, err := filesFrom(fstest.MapFS{"dist/.keep": {}}); err == nil {
		t.Error("a dist directory without index.html must be reported as not built")
	}
	sub, err := filesFrom(fstest.MapFS{"dist/index.html": {Data: []byte("<!doctype html>")}})
	if err != nil {
		t.Fatalf("built frontend rejected: %v", err)
	}
	if _, err := sub.Open("index.html"); err != nil {
		t.Errorf("index.html not reachable from the sub filesystem: %v", err)
	}
}
