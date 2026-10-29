package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

var (
	sitemapPaths   = []string{"/", "/procesos", "/memoria", "/interrupciones", "/concurrencia", "/x86", "/glosario"}
	robotsDisallow = []string{"/admin", "/login", "/perfil", "/preferencias", "/sesiones", "/full", "/v/"}
)

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if isSecureRequest(r) {
		scheme = "https"
	}
	return scheme + "://" + publicHost(r)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for _, p := range robotsDisallow {
		fmt.Fprintf(&b, "Disallow: %s\n", p)
	}
	fmt.Fprintf(&b, "\nSitemap: %s/sitemap.xml\n", publicBaseURL(r))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	set := sitemapURLSet{URLs: make([]sitemapURL, len(sitemapPaths))}
	for i, p := range sitemapPaths {
		set.URLs[i] = sitemapURL{Loc: base + p}
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(set)
}
