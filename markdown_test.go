package main

import (
	"strings"
	"testing"
)

func TestReadFrontMatterReportsOncePerValue(t *testing.T) {
	text := "---\ncss: 'p { margin: 0; }'\n---\n# Notes\n"
	if css, _, _ := readFrontMatter("reported.md", text); css == "" {
		t.Fatal("the first read found no CSS")
	}
	if css, _, _ := readFrontMatter("reported.md", text); css == "" {
		t.Fatal("the second read found no CSS")
	}
	reportedFrontMatter.Lock()
	defer reportedFrontMatter.Unlock()
	if reportedFrontMatter.css["reported.md"] != "p { margin: 0; }" {
		t.Errorf("reported CSS = %q", reportedFrontMatter.css["reported.md"])
	}
}

func TestAsMarkdownDocumentWrapsNonMarkdown(t *testing.T) {
	css, _, markdown := asMarkdownDocument("script.py", "print('hi')\n")
	if css != "" {
		t.Errorf("css = %q, want empty", css)
	}
	if !strings.HasPrefix(markdown, "# script.py\n\n```python\n") {
		t.Errorf("markdown = %q, want the file in a python code block", markdown)
	}
}

func TestAsMarkdownDocumentMarkdownKeepsItsText(t *testing.T) {
	_, _, markdown := asMarkdownDocument("notes.MARKDOWN", "# Notes\n")
	if markdown != "# Notes\n" {
		t.Errorf("markdown = %q, want the text unchanged", markdown)
	}
}
