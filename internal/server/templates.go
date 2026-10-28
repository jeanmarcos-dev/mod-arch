package server

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/jeanmarcos-dev/mod-arch/internal/server/i18n"
)

//go:embed templates/*.html templates/*.css
var templateFS embed.FS

// templateFuncs is shared by every server-rendered HTML so they all support
// `{{ T .Lang "key" }}`. T resolves through the i18n catalog with optional
// key=value pairs as interpolation params, e.g.
//
//	{{ T .Lang "full.lead_stat" "full" .Full "max" .MaxPublic }}
var templateFuncs = template.FuncMap{
	"T": func(lang, code string, args ...any) string {
		params := map[string]string{}
		for i := 0; i+1 < len(args); i += 2 {
			k, ok := args[i].(string)
			if !ok {
				continue
			}
			params[k] = sprintAny(args[i+1])
		}
		return i18n.T(lang, code, params)
	},
}

func sprintAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	default:
		return ""
	}
}

// mustParseTemplate parses a single HTML file from templateFS or panics.
// Each top-level template is parsed in isolation so they remain independent.
// Every parsed template gets the shared FuncMap so the `T` helper is always
// available inside server-rendered HTML.
func mustParseTemplate(name, filePath string) *template.Template {
	b, err := fs.ReadFile(templateFS, filePath)
	if err != nil {
		panic("server: read template " + filePath + ": " + err.Error())
	}
	return template.Must(template.New(name).Funcs(templateFuncs).Parse(string(b)))
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
