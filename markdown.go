package main

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/skoszewski/serve-markdown/v3/render"
)

// The CSS last reported for each file name, so an unchanged document is reported once.
var reportedFrontMatter = struct {
	sync.Mutex
	css map[string]string
}{css: map[string]string{}}

// readFrontMatter splits a Markdown file's front matter off and returns the CSS it declares
// and what it says of the document, with the body, as render.FrontMatter reads them.
//
// What was found is logged, and logged again only once it differs, since the document is
// re-read on every poll of /content.
func readFrontMatter(fileName, text string) (css string, info render.DocumentInfo, body string) {
	css, info, body, err := render.FrontMatter(text)
	if len(body) == len(text) {
		reportedFrontMatter.Lock()
		delete(reportedFrontMatter.css, fileName)
		reportedFrontMatter.Unlock()
		return css, info, body
	}

	reportedFrontMatter.Lock()
	reported, seen := reportedFrontMatter.css[fileName]
	changed := !seen || reported != css
	if changed {
		reportedFrontMatter.css[fileName] = css
	}
	reportedFrontMatter.Unlock()

	if changed {
		logInfo("  Front matter found in %s%s%s", colorCyan, fileName, colorReset)
		if css != "" {
			logInfo("  CSS injected from the front matter: %d lines", len(strings.Split(strings.TrimSuffix(css, "\n"), "\n")))
		}
		if err != nil {
			logInfo("  %sthe block does not parse as YAML, so its keys are not read%s", colorYellow, colorReset)
		}
	}
	return css, info, body
}

// asMarkdownDocument returns text as Markdown with the CSS and the description its front
// matter holds, wrapping a non-Markdown file in a code block.
func asMarkdownDocument(fileName, text string) (css string, info render.DocumentInfo, markdown string) {
	if render.MarkdownExtensions[strings.ToLower(filepath.Ext(fileName))] {
		return readFrontMatter(fileName, text)
	}
	return "", render.DocumentInfo{}, render.CodeDocument(fileName, text)
}
