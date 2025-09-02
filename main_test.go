package main

import (
	"strings"
	"testing"
)

func TestDetectHTMLVersion(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"HTML5", "<!DOCTYPE html><html><head></head><body></body></html>", "HTML5"},
		{"HTML4 Strict", "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://www.w3.org/TR/html4/strict.dtd\">", "HTML 4.01 Strict"},
		{"XHTML 1.0 Transitional", "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Transitional//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd\">", "XHTML 1.0 Transitional"},
		{"Unknown", "<html><head></head><body></body></html>", "Unknown (no <!DOCTYPE>)"},
	}
	for _, c := range cases {
		got := detectHTMLVersion([]byte(c.html))
		if got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestSameHost(t *testing.T) {
	base, _ := normalizeURL("https://example.com")
	same, _ := normalizeURL("https://www.example.com/page")
	diff, _ := normalizeURL("https://other.com")

	if !sameHost(base, same) {
		t.Errorf("expected hosts %s and %s to match", base, same)
	}
	if sameHost(base, diff) {
		t.Errorf("expected hosts %s and %s to differ", base, diff)
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "https://example.com"},
		{"http://test.com", "http://test.com"},
		{"https://secure.org/path", "https://secure.org/path"},
	}

	for _, tt := range tests {
		got, err := normalizeURL(tt.input)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tt.input, err)
		}
		if !strings.HasPrefix(got.String(), tt.want) {
			t.Errorf("expected %s, got %s", tt.want, got)
		}
	}
}
