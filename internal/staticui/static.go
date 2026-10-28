// Package staticui embebe el frontend Vue ya construido en el binario Go.
//
// El directorio dist/ se genera con `cd web && npm run build`.
package staticui

import (
	"embed"
	"fmt"
	"io/fs"
)

// The all: prefix is required so Vite-generated chunks whose filenames start
// with "_" (vue export helpers, async chunks) are not silently skipped by the
// default //go:embed dotfile/underscore filter.
//
//go:embed all:dist
var distEmbed embed.FS

// Files devuelve el filesystem con los archivos del frontend listos para servir.
// Devuelve nil + error si el embed está vacío (significaría que nunca se buildeó el web).
func Files() (fs.FS, error) {
	return filesFrom(distEmbed)
}

func filesFrom(fsys fs.FS) (fs.FS, error) {
	sub, err := fs.Sub(fsys, "dist")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, fmt.Errorf("staticui: frontend not built: %w", err)
	}
	return sub, nil
}
