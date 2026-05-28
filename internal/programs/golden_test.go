package programs

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate testdata/demos.golden from current demo state")

// TestDemosGolden pins every byte, every line of Source and every SourceMap
// entry of every demo. A change anywhere makes the test fail with a clear diff
// telling the human which Proc moved. Run with `-update` to accept the change.
//
// Hashes are kept separate (bytes / source / sourcemap) so that, for instance,
// a comment-only edit to programs.go fails the source row but leaves the bytes
// row green — which is exactly what we want for the cleanup PR.
func TestDemosGolden(t *testing.T) {
	manifest := buildDemoManifest()
	goldenPath := filepath.Join("testdata", "demos.golden")

	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(manifest), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("regenerated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run `go test -update` to create it): %v", err)
	}

	if string(want) == manifest {
		return
	}

	wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimRight(manifest, "\n"), "\n")
	t.Errorf("demo manifest drift — review the diff below and re-run with `-update` if intentional.\n\n%s",
		unifiedDiff(wantLines, gotLines))
}

// buildDemoManifest renders one line per Proc, sorted by demo name then proc
// name, with the format:
//
//	<demo>/<proc>  bytes=<sha>  source=<sha>  sourcemap=<sha>
//
// The single-pass format keeps diffs minimal and reviewable.
func buildDemoManifest() string {
	type row struct {
		key            string
		bytesSha       string
		sourceSha      string
		sourceMapSha   string
		bytesLen       int
		sourceLines    int
		sourceMapLines int
	}

	var rows []row
	for _, d := range Demos() {
		for _, p := range d.Procs {
			rows = append(rows, row{
				key:            d.Name + "/" + p.Name,
				bytesSha:       hashBytes(p.Bytes),
				sourceSha:      hashBytes([]byte(p.Source)),
				sourceMapSha:   hashSourceMap(p.SourceMap),
				bytesLen:       len(p.Bytes),
				sourceLines:    strings.Count(p.Source, "\n"),
				sourceMapLines: len(p.SourceMap),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })

	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b,
			"%-48s bytes=%s(%d)  source=%s(%dL)  sourcemap=%s(%d)\n",
			r.key, r.bytesSha, r.bytesLen, r.sourceSha, r.sourceLines, r.sourceMapSha, r.sourceMapLines)
	}
	return b.String()
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func hashSourceMap(m []SourceLine) string {
	var b strings.Builder
	for _, sl := range m {
		fmt.Fprintf(&b, "%08x|%d|%s\n", sl.Offset, sl.CLine, sl.Mnemonic)
	}
	return hashBytes([]byte(b.String()))
}

// unifiedDiff renders a compact line-by-line diff suitable for test failure
// output. Standard library has no diff, and a 3-line ad-hoc implementation
// is enough since manifests are short (≤ 50 lines).
func unifiedDiff(want, got []string) string {
	wantSet := map[string]int{}
	for _, l := range want {
		wantSet[l]++
	}
	gotSet := map[string]int{}
	for _, l := range got {
		gotSet[l]++
	}

	var out strings.Builder
	for _, l := range want {
		if gotSet[l] == 0 {
			fmt.Fprintf(&out, "- %s\n", l)
		}
	}
	for _, l := range got {
		if wantSet[l] == 0 {
			fmt.Fprintf(&out, "+ %s\n", l)
		}
	}
	if out.Len() == 0 {
		out.WriteString("(no line-level diff but content differs — check whitespace)\n")
	}
	return out.String()
}
