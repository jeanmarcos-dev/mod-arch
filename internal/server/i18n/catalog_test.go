package i18n

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

// TestCatalogParity guards against a one-sided edit: every code present in ES
// must be present in EN, and vice versa. Adding a new entry to one locale
// without the other will fail this test before it ships.
func TestCatalogParity(t *testing.T) {
	t.Helper()
	es := keys(Catalog["es"])
	en := keys(Catalog["en"])
	sort.Strings(es)
	sort.Strings(en)
	if len(es) != len(en) {
		t.Fatalf("locale key counts differ: es=%d en=%d", len(es), len(en))
	}
	for i := range es {
		if es[i] != en[i] {
			t.Fatalf("missing translation at index %d: es=%q en=%q", i, es[i], en[i])
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestInterpolate(t *testing.T) {
	got := Interpolate("hello {name}, you have {n} items", map[string]string{
		"name": "world",
		"n":    "3",
	})
	if got != "hello world, you have 3 items" {
		t.Fatalf("unexpected interpolation: %q", got)
	}
}

func TestTFallsBackToDefaultLocale(t *testing.T) {
	// French is not supported, so T should fall back to ES.
	got := T("fr", CodeAuthInvalidJSON, nil)
	want := Catalog["es"][CodeAuthInvalidJSON]
	if got != want {
		t.Fatalf("expected fallback to ES, got %q", got)
	}
}

func TestResolveLangFromCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: LangCookie, Value: "en"})
	if got := ResolveLang(r); got != "en" {
		t.Fatalf("expected en from cookie, got %q", got)
	}
}

func TestResolveLangAcceptLanguage(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")
	if got := ResolveLang(r); got != "en" {
		t.Fatalf("expected en from Accept-Language, got %q", got)
	}
}

func TestResolveLangDefaultsToSpanish(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := ResolveLang(r); got != DefaultLocale {
		t.Fatalf("expected default %q, got %q", DefaultLocale, got)
	}
}
