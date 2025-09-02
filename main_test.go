package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// --- HTML Detections -----------------------------------------------------

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

// --- Headings incl. ARIA -----------------------------------------------------
func TestCountHeadings_IncludesARIA(t *testing.T) {
	html := `
	<!doctype html><html><body>
	<h1>a</h1><h2>b</h2><h3>c</h3>
	<div role="heading" aria-level="1">x</div>
	<div role="heading" aria-level="3">y</div>
	<div role="heading" aria-level="6">z</div>
	</body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	got := countHeadings(doc)

	want := map[int]int{1: 2, 2: 1, 3: 2, 4: 0, 5: 0, 6: 1}
	for lvl := 1; lvl <= 6; lvl++ {
		if got[lvl] != want[lvl] {
			t.Errorf("h%d: want %d got %d", lvl, want[lvl], got[lvl])
		}
	}
}

// --- URL normalization & sameHost -------------------------------------------
func TestNormalizeURL_Errors(t *testing.T) {
	bad := []string{"://bad", "ftp://example.com", "http://"}
	for _, in := range bad {
		if _, err := normalizeURL(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestSameHost_WWWEquivalence(t *testing.T) {
	a, _ := normalizeURL("https://example.com")
	b, _ := normalizeURL("https://www.example.com/path")
	if !sameHost(a, b) {
		t.Fatalf("expected %s and %s to be same host (www equivalence)", a, b)
	}
}

// --- Login detection heuristic ----------------------------------------------
func TestAnalyze_LoginDetection(t *testing.T) {
	base, _ := normalizeURL("https://example.com")
	html := `
	<!doctype html><html><body>
	  <form><input type="text" name="user"><input type="password" name="pwd"></form>
	</body></html>`
	res, err := analyzeFromHTML(base, html)
	if err != nil {
		t.Fatalf("analyze error: %v", err)
	}
	if !res.HasLogin {
		t.Fatalf("expected HasLogin=true")
	}
}

func TestAnalyze_LoginDetectionByNameOnly(t *testing.T) {
	base, _ := normalizeURL("https://example.com")
	html := `
	<!doctype html><html><body>
	  <form><input type="text" name="username"><input type="text" name="user_password"></form>
	</body></html>`
	res, err := analyzeFromHTML(base, html)
	if err != nil {
		t.Fatalf("analyze error: %v", err)
	}
	if !res.HasLogin {
		t.Fatalf("expected HasLogin=true (name contains 'password')")
	}
}

// --- Internal vs External links ---------------------------------------------
func TestAnalyze_InternalExternalCounts(t *testing.T) {
	base, _ := normalizeURL("https://example.com")
	html := `
	<!doctype html><html><body>
	  <a href="/local">rel</a>
	  <a href="https://example.com/abs">abs same</a>
	  <a href="https://www.example.com/www">www same</a>
	  <a href="https://other.com/">external</a>
	  <a href="mailto:test@example.com">mail</a>
	  <a href="javascript:void(0)">js</a>
	  <a href="#frag">frag</a>
	</body></html>`
	res, err := analyzeFromHTML(base, html)
	if err != nil {
		t.Fatalf("analyze error: %v", err)
	}
	if res.InternalLinks != 3 || res.ExternalLinks != 1 {
		t.Fatalf("want internal=3 external=1, got %d/%d", res.InternalLinks, res.ExternalLinks)
	}
}

// --- Fetch + status via httptest (no internet) -------------------------------
func TestFetch_StatusAndRedirect(t *testing.T) {
	// final 200 server
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("<!doctype html><title>OK</title>"))
	}))
	t.Cleanup(ok.Close)

	// redirecting server -> to ok.URL
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ok.URL, http.StatusMovedPermanently) // 301
	}))
	t.Cleanup(redirect.Close)

	// Use our fetch to follow redirect
	resp, body, err := fetch(t.Context(), redirect.URL)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected final status 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<title>OK</title>") {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

// --- helpers ----------------------------------------------------------------

// analyzeFromHTML lets us bypass real fetch in unit tests.
func analyzeFromHTML(base *url.URL, html string) (*analysisResult, error) {
	_, _ = goquery.NewDocumentFromReader(strings.NewReader(html))
	// emulate what analyze() does internally using the parsed document:
	// We'll reuse the real 'analyze' by passing body bytes to it.
	return analyze(tContext(), base, []byte(html))
}

// tContext returns a background-like context for tests.
func tContext() context.Context { return context.Background() }
