// Package staticui embebe el frontend Vue ya construido en el binario Go.
//
// El directorio dist/ se genera con `cd web && npm run build`. Está versionado
// para que `go build` funcione sin Node instalado, pero el Makefile lo regenera
// antes de cada build.
package staticui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distEmbed embed.FS

// Files devuelve el filesystem con los archivos del frontend listos para servir.
// Devuelve nil + error si el embed está vacío (significaría que nunca se buildeó el web).
func Files() (fs.FS, error) {
	sub, err := fs.Sub(distEmbed, "dist")
	if err != nil {
		return nil, err
	}
	return sub, nil
}
