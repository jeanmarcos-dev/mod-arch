package server

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed templates/*.html templates/*.css templates/*.js
var templateFS embed.FS

// mustParseTemplate parses a single HTML file from templateFS or panics.
// Each top-level template is parsed in isolation so they remain independent.
func mustParseTemplate(name, filePath string) *template.Template {
	b, err := fs.ReadFile(templateFS, filePath)
	if err != nil {
		panic("server: read template " + filePath + ": " + err.Error())
	}
	return template.Must(template.New(name).Parse(string(b)))
}

// readStaticAsset returns the raw bytes of a file from templateFS. It panics
// if the file is missing; the caller is expected to use a path that is known
// to exist at compile time.
func readStaticAsset(filePath string) []byte {
	b, err := fs.ReadFile(templateFS, filePath)
	if err != nil {
		panic("server: read static asset " + filePath + ": " + err.Error())
	}
	return b
}

// assetContentTypes maps allowed asset extensions to their HTTP content types.
// Only entries listed here are served by handleAsset.
var assetContentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "application/javascript; charset=utf-8",
}

// handleAsset serves CSS and JavaScript files embedded under templates/.
// The path is taken from the {name} path value and validated against the
// extension whitelist; any other request returns 404.
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ext := strings.ToLower(path.Ext(name))
	ct, ok := assetContentTypes[ext]
	if !ok || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	body, err := fs.ReadFile(templateFS, "templates/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(body)
}
