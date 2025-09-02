# Webpage Analyzer (Go)

[![Go Build & Test](https://github.com/jestress/webanalyzer/actions/workflows/build.yaml/badge.svg)](https://github.com/jestress/webanalyzer/actions/workflows/build.yaml)

[![Go Lint](https://github.com/jestress/webanalyzer/actions/workflows/lint.yaml/badge.svg)](https://github.com/jestress/webanalyzer/actions/workflows/lint.yaml)

This project implements a small web application in **Go** that analyzes a given web page URL.  
It was developed as a coding task and demonstrates HTML parsing, concurrent link checking, and server-side rendering.

---

## Features

- Accepts a URL input via a form and fetches the page
- Displays:
  - **HTTP status code** and **final URL** (after redirects)
  - **HTML version** (HTML5, HTML 4.01 variants, XHTML 1.0/1.1, or "Unknown")
  - **Page title**
  - **Heading counts** (`<h1>` through `<h6>`, plus ARIA `role="heading"`)
  - **Login form detection** (password field heuristics)
  - **Link summary**:
    - Internal vs external link counts
    - Inaccessible links (status ≥ 400 or network error)
    - Capped link checks (to avoid hammering)
- Shows friendly error messages if the page cannot be fetched

---

## Quick Start

```bash
# clone repo
git clone https://github.com/jestress/webanalyzer
cd webanalyzer

# ensure Go 1.24+ is installed
go mod tidy

# run locally
go run .
```

Then open [http://localhost:8080](http://localhost:8080) in your browser.

---

## File Structure

```
.
├── analyzer.html     # Main Page
├── consts.go         # Constants
├── data.go           # Structs
├── go.mod
├── go.sum
└── main.go           # Go server & analyzer logic
```

---

## Trade-offs & Limitations

### HTML Parsing
- Uses [goquery](https://github.com/PuerkitoBio/goquery) to parse the raw HTML.
- **JavaScript is not executed.** If the site is a JS-heavy SPA, headings, titles, or even content may not exist in the raw HTML.  
  - Example: `<h1>` might only appear after React renders client-side, so it will **not** be counted.
  - **Trade-off:** keeps the app light and dependency-free. Executing JS would require a headless browser (e.g., `chromedp`, `rod`, Playwright).
  - Analyzing "https://w3schools.com" would yield correct headings count, but "https://youtube.com" would not, due to JS-heavy difference.

### Link Checking
- We check a **capped number** of links (default 150) to prevent overloading target sites.
- Uses `HEAD` requests first, falling back to `GET` if needed.
- **Trade-off:** Adds outbound traffic and delays, but gives realistic reachability data.

### HTTP Status Reporting
- The app shows the **status code of the user-provided URL** (200, 301, 404, etc.).
- For broken links inside the page, only the count is shown (to avoid huge output).  
  You can extend it to display each broken link + status.

### Login Form Detection
- Simple heuristics: checks for `<input type="password">` or field names containing `"password"`.
- May miss custom authentication UIs.

### HTML Version Detection
- Based on `<!DOCTYPE>` declaration.  
- Many modern pages omit it, which results in "Unknown".
- **Trade-off:** avoids over-engineering; sufficient for most static HTML.

### Performance
- Concurrent link checks (12 workers by default).
- Overall timeout budget of ~45s for an analysis run.
- Capped body size (~4MB) to prevent downloading very large pages.

### Security Considerations
- No sandboxing of requested pages (it directly fetches given URLs).  
  If deployed publicly, you should:
  - Add rate limiting.
  - Restrict internal/private IP ranges (to prevent SSRF).
  - Identify yourself in `User-Agent`.

---

## Possible Improvements

- Render JS pages via `chromedp` or Playwright for more accurate heading detection.
- Show the full redirect chain (301 → 302 → 200).
- Cache link check results per domain to reduce load.
- Export results as JSON/CSV.
- Add unit tests for parsers and utilities.
- UI polish: filter/sort headings and links.

---

## Example Sites To Try

- https://w3schools.com (Not-JS heavy, yields healthy results)
- https://youtube.com (JS-heavy, headings might not be parsed)
- https://google.com
