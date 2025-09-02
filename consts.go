package main

import (
	"regexp"
	"strings"
	"time"
)

const (
	defaultAddr        = ":8080"
	maxLinksToCheck    = 150 // hard cap to avoid hammering big pages
	linkCheckWorkers   = 12  // concurrency for link checks
	perRequestTimeout  = 8 * time.Second
	totalAnalyzeBudget = 45 * time.Second
)

var reDoctype = regexp.MustCompile(`(?is)<!DOCTYPE\s+html(?:\s+PUBLIC\s+"([^"]*)"(?:\s+"[^"]*")?)?.*>`)
var reXHTML = regexp.MustCompile(`(?i)xhtml`)
var reStrict = regexp.MustCompile(`(?i)strict`)
var reTrans = regexp.MustCompile(`(?i)transitional`)
var reFrame = regexp.MustCompile(`(?i)frameset`)

// detectHTMLVersion inspects the HTML doctype to determine the HTML version.
// If no doctype is found, it returns "Unknown (no <!DOCTYPE>)".
func detectHTMLVersion(html []byte) string {
	m := reDoctype.FindSubmatch(html)
	if m == nil {
		// No doctype usually implies quirks, but modern pages often omit explicit doctype in fragments.
		return "Unknown (no <!DOCTYPE>)"
	}
	publicID := strings.ToLower(string(m[1]))
	if publicID == "" {
		return "HTML5"
	}
	switch {
	case reXHTML.MatchString(publicID):
		switch {
		case strings.Contains(publicID, "1.1"):
			return "XHTML 1.1"
		case strings.Contains(publicID, "1.0"):
			switch {
			case reStrict.MatchString(publicID):
				return "XHTML 1.0 Strict"
			case reTrans.MatchString(publicID):
				return "XHTML 1.0 Transitional"
			case reFrame.MatchString(publicID):
				return "XHTML 1.0 Frameset"
			default:
				return "XHTML 1.0"
			}
		default:
			return "XHTML (unknown minor)"
		}
	default:
		// HTML 4.01 variants
		switch {
		case reStrict.MatchString(publicID) && strings.Contains(publicID, "4.01"):
			return "HTML 4.01 Strict"
		case reTrans.MatchString(publicID) && strings.Contains(publicID, "4.01"):
			return "HTML 4.01 Transitional"
		case reFrame.MatchString(publicID) && strings.Contains(publicID, "4.01"):
			return "HTML 4.01 Frameset"
		}
	}
	return "Unknown (doctype present)"
}
