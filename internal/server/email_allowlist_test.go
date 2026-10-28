package server

import (
	"slices"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	cases := []struct{ in, out string }{
		{"ucv.edu.ve", "ucv.edu.ve"},
		{"  UCV.EDU.VE  ", "ucv.edu.ve"},
		{"@example.com", "example.com"},
		{"@@example.com", "example.com"}, // double-prefix collapses through TrimPrefix once; remaining @ stays
		{"plain", ""},                    // no dot
		{"with space.com", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeDomain(c.in)
		if got != c.out {
			t.Errorf("normalizeDomain(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestParseList(t *testing.T) {
	raw := "ucv.edu.ve, @example.com\nfoo.bar\n;\n\nucv.edu.ve\nINVALID NAME"
	got := parseList(raw, normalizeDomain)
	want := []string{"ucv.edu.ve", "example.com", "foo.bar"}
	if !slices.Equal(got, want) {
		t.Errorf("parseList = %v, want %v", got, want)
	}
}

func TestEmailAllowlist_Permits(t *testing.T) {
	cases := []struct {
		name    string
		domains []string
		emails  []string
		email   string
		want    bool
	}{
		{"empty bypasses", nil, nil, "anyone@example.com", true},
		{"empty email always allowed", []string{"x.com"}, nil, "", true},
		{"domain match", []string{"ucv.edu.ve"}, nil, "carlos@ucv.edu.ve", true},
		{"domain mismatch", []string{"ucv.edu.ve"}, nil, "carlos@gmail.com", false},
		{"explicit email match", []string{"ucv.edu.ve"}, []string{"profe@otro.com"}, "profe@otro.com", true},
		{"explicit email mismatch", nil, []string{"profe@otro.com"}, "alguien@otro.com", false},
		{"case-normalized lookups", []string{"ucv.edu.ve"}, nil, "carlos@ucv.edu.ve", true},
		{"malformed email rejected when enforcing", []string{"ucv.edu.ve"}, nil, "no-at-sign", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := EmailAllowlist{Domains: c.domains, Emails: c.emails}
			if got := a.Permits(c.email); got != c.want {
				t.Errorf("Permits(%q) with domains=%v emails=%v = %v, want %v",
					c.email, c.domains, c.emails, got, c.want)
			}
		})
	}
}
