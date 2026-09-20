package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

var markdownExtensions = map[string]bool{".md": true, ".markdown": true}

var codeFenceLanguages = map[string]string{
	".ps1":    "powershell",
	".py":     "python",
	".rb":     "ruby",
	".sh":     "bash",
	".tf":     "hcl",
	".tfvars": "hcl",
	".txt":    "plaintext",
	".yml":    "yaml",
	".zsh":    "bash",
}

// A front matter block opens on the file's first line and closes on a "---" or "..." line.
var frontMatterPattern = regexp.MustCompile(`(?s)\A---[ \t]*\r?\n((?:.*?\r?\n)??)(?:---|\.\.\.)[ \t]*(?:\r?\n|\z)`)

var backtickRunPattern = regexp.MustCompile("`+")

// The CSS last reported for each file name, so an unchanged document is reported once.
var reportedFrontMatter = struct {
	sync.Mutex
	css map[string]string
}{css: map[string]string{}}

// readFrontMatter splits a Markdown file's front matter off and returns the CSS it declares
// with the body.
//
// The front matter is dropped from the document, since Markdown renderers show it as a rule
// followed by its raw keys. Its "css" key styles the page; a block that is not a mapping
// holding a "css" string leaves the document with the server's own styling alone.
//
// What was found is logged, and logged again only once it differs, since the document is
// re-read on every poll of /content.
func readFrontMatter(fileName, text string) (css, body string) {
	match := frontMatterPattern.FindStringSubmatchIndex(text)
	if match == nil {
		reportedFrontMatter.Lock()
		delete(reportedFrontMatter.css, fileName)
		reportedFrontMatter.Unlock()
		return "", text
	}

	body = text[match[1]:]
	note := ""
	var frontMatter any
	if err := yaml.Unmarshal([]byte(text[match[2]:match[3]]), &frontMatter); err != nil {
		note = "the block does not parse as YAML, so the 'css' key is not read"
	} else if mapping, isMapping := frontMatter.(map[string]any); isMapping {
		if declared, isString := mapping["css"].(string); isString {
			css = declared
		}
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
		if note != "" {
			logInfo("  %s%s%s", colorYellow, note, colorReset)
		}
	}
	return css, body
}

// asMarkdownDocument returns text as Markdown with the CSS it asks for, wrapping a
// non-Markdown file in a code block.
//
// The code fence is made longer than the longest run of backticks in the file, so a file that
// itself contains fences stays inside its own block.
func asMarkdownDocument(fileName, text string) (css, markdown string) {
	suffix := strings.ToLower(filepath.Ext(fileName))
	if markdownExtensions[suffix] {
		return readFrontMatter(fileName, text)
	}

	language, known := codeFenceLanguages[suffix]
	if !known {
		language = strings.TrimPrefix(suffix, ".")
	}

	longest := 0
	for _, run := range backtickRunPattern.FindAllString(text, -1) {
		if len(run) > longest {
			longest = len(run)
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return "", fmt.Sprintf("# %s\n\n%s%s\n%s\n%s\n", fileName, fence, language, strings.TrimRight(text, "\n"), fence)
}
