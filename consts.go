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

var reDoctypeFull = regexp.MustCompile(`(?is)<!DOCTYPE\s+html(?:\s+PUBLIC\s+"([^"]*)"(?:\s+"([^"]*)")?)?.*>`)

// detectHTMLVersion inspects the HTML doctype to determine the HTML version.
// If no doctype is found, it returns "Unknown (no <!DOCTYPE>)".
func detectHTMLVersion(html []byte) string {
	m := reDoctypeFull.FindSubmatch(html)
	if m == nil {
		return "Unknown (no <!DOCTYPE>)"
	}
	publicID := strings.ToLower(string(m[1]))
	systemID := strings.ToLower(string(m[2]))

	if publicID == "" && systemID == "" {
		return "HTML5"
	}

	// Check for XHTML
	if strings.Contains(publicID, "xhtml") {
		switch {
		case strings.Contains(publicID, "1.1"):
			return "XHTML 1.1"
		case strings.Contains(publicID, "1.0"):
			switch {
			case strings.Contains(publicID, "strict") || strings.Contains(systemID, "strict"):
				return "XHTML 1.0 Strict"
			case strings.Contains(publicID, "transitional") || strings.Contains(systemID, "transitional"):
				return "XHTML 1.0 Transitional"
			case strings.Contains(publicID, "frameset") || strings.Contains(systemID, "frameset"):
				return "XHTML 1.0 Frameset"
			default:
				return "XHTML 1.0"
			}
		}
	}

	// Check for HTML 4.01
	if strings.Contains(publicID, "4.01") || strings.Contains(systemID, "4.01") {
		switch {
		case strings.Contains(publicID, "strict") || strings.Contains(systemID, "strict"):
			return "HTML 4.01 Strict"
		case strings.Contains(publicID, "transitional") || strings.Contains(systemID, "transitional"):
			return "HTML 4.01 Transitional"
		case strings.Contains(publicID, "frameset") || strings.Contains(systemID, "frameset"):
			return "HTML 4.01 Frameset"
		default:
			return "HTML 4.01"
		}
	}

	return "Unknown (doctype present)"
}
