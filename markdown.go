package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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

// documentInfo is what a document's front matter says of it, shown in the page's top row.
type documentInfo struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Date   string `json:"date"`
}

// readFrontMatter splits a Markdown file's front matter off and returns the CSS it declares
// and what it says of the document, with the body.
//
// The front matter is dropped from the document, since Markdown renderers show it as a rule
// followed by its raw keys. Its "css" key styles the page, and its "title", "author" and
// "date" keys describe the document; a key that is not a single value is not read, and a
// block that is not a mapping leaves the document with the server's own styling alone.
//
// What was found is logged, and logged again only once it differs, since the document is
// re-read on every poll of /content.
func readFrontMatter(fileName, text string) (css string, info documentInfo, body string) {
	match := frontMatterPattern.FindStringSubmatchIndex(text)
	if match == nil {
		reportedFrontMatter.Lock()
		delete(reportedFrontMatter.css, fileName)
		reportedFrontMatter.Unlock()
		return "", documentInfo{}, text
	}

	body = text[match[1]:]
	note := ""
	var frontMatter any
	if err := yaml.Unmarshal([]byte(text[match[2]:match[3]]), &frontMatter); err != nil {
		note = "the block does not parse as YAML, so its keys are not read"
	} else if mapping, isMapping := frontMatter.(map[string]any); isMapping {
		if declared, isString := mapping["css"].(string); isString {
			css = declared
		}
		info = documentInfo{Title: frontMatterValue(mapping["title"]),
			Author: frontMatterValue(mapping["author"]), Date: frontMatterValue(mapping["date"])}
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
	return css, info, body
}

// frontMatterValue returns a single front matter value as the text it is shown as, a date
// written without a time as the date alone, and nothing for a list, a mapping or no value.
func frontMatterValue(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case time.Time:
		if hour, minute, second := value.Clock(); hour+minute+second+value.Nanosecond() == 0 {
			return value.Format(time.DateOnly)
		}
		return value.Format("2006-01-02 15:04")
	case int, float64, bool:
		return fmt.Sprint(value)
	}
	return ""
}

// asMarkdownDocument returns text as Markdown with the CSS and the description its front
// matter holds, wrapping a non-Markdown file in a code block.
//
// The code fence is made longer than the longest run of backticks in the file, so a file that
// itself contains fences stays inside its own block.
func asMarkdownDocument(fileName, text string) (css string, info documentInfo, markdown string) {
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
	return "", documentInfo{}, fmt.Sprintf("# %s\n\n%s%s\n%s\n%s\n", fileName, fence, language, strings.TrimRight(text, "\n"), fence)
}
