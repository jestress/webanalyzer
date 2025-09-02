package main

import "net/url"

// pageData holds all data related to a single page analysis session.
type pageData struct {
	InputURL     string
	CanonicalURL string
	HTTPStatus   int
	Error        string
	Result       *analysisResult
	PerRequestTO int
	Budget       int
}

// analysisResult holds the results of analyzing a single page.
type analysisResult struct {
	HTMLVersion       string
	Title             string
	Headings          map[int]int // level => count
	InternalLinks     int
	ExternalLinks     int
	InaccessibleLinks int
	CheckedLinks      int
	CheckedLinksCap   int
	HasLogin          bool
}

// link represents a hyperlink found on the page, along with whether it's internal or external.
type link struct {
	URL        *url.URL
	IsInternal bool
}
