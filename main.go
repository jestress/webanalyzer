package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var pageTmpl *template.Template

func init() {
	var err error
	pageTmpl, err = template.ParseFiles("analyzer.html")
	if err != nil {
		panic(fmt.Errorf("failed to parse template: %w", err))
	}
}

func main() {
	m := http.NewServeMux()
	m.HandleFunc("/", index)
	m.HandleFunc("/analyze", handleAnalyze)

	s := &http.Server{
		Addr:              defaultAddr,
		Handler:           handlerMiddleware(m),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("Listening on %s …\n", defaultAddr)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

// handlerMiddleware logs requests and their durations.
func handlerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			d := time.Since(start)
			fmt.Printf("%s %s (%s)\n", r.Method, r.URL.Path, d)
		}()
		next.ServeHTTP(w, r)
	})
}

// index serves the main page with the input form.
func index(w http.ResponseWriter, r *http.Request) {
	_ = pageTmpl.Execute(w, pageData{
		PerRequestTO: int(perRequestTimeout.Seconds()),
		Budget:       int(totalAnalyzeBudget.Seconds()),
	})
}

// handleAnalyze processes the URL analysis request.
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeErr(w, "", 0, fmt.Errorf("bad URL form: %w", err))
		return
	}

	raw := strings.TrimSpace(r.Form.Get("u"))
	if raw == "" {
		writeErr(w, "", 0, errors.New("please provide a URL"))
		return
	}
	url, err := normalizeURL(raw)
	if err != nil {
		writeErr(w, raw, 0, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), totalAnalyzeBudget)
	defer cancel()

	status := 0
	finalURL := url.String()
	resp, body, fetchErr := fetch(ctx, finalURL)
	if fetchErr != nil {
		if resp != nil {
			status = resp.StatusCode
			// net/http follows redirects; show the final URL if available
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.String()
			}
		}
		writeErr(w, finalURL, status, fetchErr)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	pgData := &pageData{
		InputURL:     raw,
		CanonicalURL: finalURL,
		HTTPStatus:   status,
		Result:       nil,
		PerRequestTO: int(perRequestTimeout.Seconds()),
		Budget:       int(totalAnalyzeBudget.Seconds()),
	}
	res, err := analyze(ctx, url, body)
	if err != nil {
		if resp != nil && resp.Request != nil && resp.Request.URL != nil {
			pgData.CanonicalURL = resp.Request.URL.String()
			pgData.HTTPStatus = resp.StatusCode
		}
		_ = pageTmpl.Execute(w, pgData)
		return
	}

	if resp.Request != nil && resp.Request.URL != nil {
		pgData.CanonicalURL = resp.Request.URL.String()
	}
	pgData.Result = res
	pgData.HTTPStatus = resp.StatusCode
	_ = pageTmpl.Execute(w, pgData)
}

// writeErr renders the error page with the given input URL, status, and error message.
func writeErr(w http.ResponseWriter, input string, status int, err error) {
	_ = pageTmpl.Execute(w, pageData{
		InputURL:     input,
		HTTPStatus:   status,
		Error:        err.Error(),
		PerRequestTO: int(perRequestTimeout.Seconds()),
		Budget:       int(totalAnalyzeBudget.Seconds()),
	})
}

// normalizeURL ensures the URL has a scheme and is valid.
func normalizeURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("missing host")
	}
	return u, nil
}

// fetch retrieves the URL content with a timeout and returns the response and body.
func fetch(ctx context.Context, u string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:              http.ProxyFromEnvironment,
			MaxIdleConns:       20,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
		Timeout: perRequestTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		// We still read body for HTML version/title if possible, but return error to satisfy the requirement.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MiB cap
		return resp, body, fmt.Errorf("non-OK status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4MiB cap for analysis
	if err != nil {
		return resp, nil, fmt.Errorf("failed reading response body: %w", err)
	}
	return resp, body, nil
}

// countHeadings counts the number of headings (h1..h6 and ARIA role="heading") in the document.
func countHeadings(doc *goquery.Document) map[int]int {
	counts := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0}

	// Standard h1..h6
	for level := 1; level <= 6; level++ {
		sel := fmt.Sprintf("h%d", level)
		counts[level] += doc.Find(sel).Length()
	}

	// ARIA role="heading" with aria-level
	doc.Find(`[role="heading"][aria-level]`).Each(func(_ int, s *goquery.Selection) {
		if lvlStr, ok := s.Attr("aria-level"); ok {
			switch strings.TrimSpace(lvlStr) {
			case "1", "2", "3", "4", "5", "6":
				lvl := int(lvlStr[0] - '0')
				counts[lvl]++
			}
		}
	})

	return counts
}

// analyze processes the HTML body to extract analysis results.
func analyze(ctx context.Context, base *url.URL, body []byte) (*analysisResult, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = "(no title)"
	}

	headings := countHeadings(doc)

	var links []link
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "#") {
			return
		}
		u2, err := base.Parse(href)
		if err != nil || u2.Scheme == "" || (u2.Scheme != "http" && u2.Scheme != "https") {
			return
		}
		isInternal := sameHost(base, u2)
		links = append(links, link{URL: u2, IsInternal: isInternal})
	})

	internalCount := 0
	externalCount := 0
	for _, l := range links {
		if l.IsInternal {
			internalCount++
		} else {
			externalCount++
		}
	}

	// Detect login form: any form with input type=password OR name contains "password"
	hasLogin := false
	doc.Find("form").EachWithBreak(func(_ int, f *goquery.Selection) bool {
		pw := f.Find(`input[type="password"]`).Length()
		if pw > 0 {
			hasLogin = true
			return false
		}
		// heuristic: input name contains 'password'
		match := false
		f.Find("input").EachWithBreak(func(_ int, in *goquery.Selection) bool {
			if name, ok := in.Attr("name"); ok && strings.Contains(strings.ToLower(name), "password") {
				match = true
				return false
			}
			return true
		})
		if match {
			hasLogin = true
			return false
		}
		return true
	})

	inacc, checked := checkLinks(ctx, links)

	ar := &analysisResult{
		HTMLVersion:       detectHTMLVersion(body),
		Title:             title,
		Headings:          headings,
		InternalLinks:     internalCount,
		ExternalLinks:     externalCount,
		InaccessibleLinks: inacc,
		CheckedLinks:      checked,
		CheckedLinksCap:   maxLinksToCheck,
		HasLogin:          hasLogin,
	}
	return ar, nil
}

// sameHost checks if two URLs share the same host (ignoring "www." prefix).
func sameHost(a, b *url.URL) bool {
	ha := strings.ToLower(a.Hostname())
	hb := strings.ToLower(b.Hostname())
	// treat "www." as same site for this scope
	trim := func(s string) string {
		return strings.TrimPrefix(s, "www.")
	}
	return trim(ha) == trim(hb)
}

// checkLinks verifies the accessibility of the provided links concurrently.
func checkLinks(ctx context.Context, links []link) (inaccessible int, checked int) {
	if len(links) == 0 {
		return 0, 0
	}

	// Prefer to check unique URLs to avoid duplicates
	unique := make([]*url.URL, 0, len(links))
	seen := make(map[string]struct{})
	for _, l := range links {
		key := l.URL.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, l.URL)
	}

	// Trim to cap
	if len(unique) > maxLinksToCheck {
		unique = unique[:maxLinksToCheck]
	}

	type result struct{ broken bool }
	jobs := make(chan *url.URL)
	results := make(chan result)
	var wg sync.WaitGroup

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:              http.ProxyFromEnvironment,
			MaxIdleConns:       40,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
			DialContext: (&net.Dialer{
				Timeout:   4 * time.Second,
				KeepAlive: 15 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 4 * time.Second,
		},
		Timeout: perRequestTimeout,
	}

	worker := func() {
		defer wg.Done()
		for u := range jobs {
			broken := !checkLink(ctx, client, u)
			select {
			case results <- result{broken: broken}:
			case <-ctx.Done():
				return
			}
		}
	}

	nw := linkCheckWorkers
	if nw > len(unique) {
		nw = len(unique)
	}
	if nw == 0 {
		return 0, 0
	}

	wg.Add(nw)
	for i := 0; i < nw; i++ {
		go worker()
	}

	go func() {
		for _, u := range unique {
			select {
			case jobs <- u:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	badCount := 0
	done := 0
	for done < len(unique) {
		select {
		case r := <-results:
			done++
			if r.broken {
				badCount++
			}
		case <-ctx.Done():
			// budget exceeded; return what we have
			close(results)
			// drain workers
			go func() {
				wg.Wait()
				close(results)
			}()
			return badCount, done
		}
	}
	wg.Wait()
	close(results)
	return badCount, done
}

// checkLink tests if a single link is accessible (HTTP 2xx or 3xx).
func checkLink(ctx context.Context, client *http.Client, u *url.URL) bool {
	ctx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()

	// Prefer HEAD, fallback to GET when HEAD not allowed
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	resp, err := client.Do(req)
	if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		return true
	}
	// Retry with GET if HEAD failed or got 405/403
	if resp != nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusForbidden {
			// treat other non-2xx as bad
			return false
		}
	}
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp2, err2 := client.Do(req2)
	if err2 != nil {
		return false
	}
	defer func() {
		_ = resp2.Body.Close()
	}()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp2.Body, 64<<10))
	return resp2.StatusCode >= 200 && resp2.StatusCode < 400
}
