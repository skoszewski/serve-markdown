package main

import (
	"strings"
	"testing"
)

func TestReadFrontMatter(t *testing.T) {
	tests := []struct {
		name string
		text string
		css  string
		body string
	}{
		{
			name: "present",
			text: "---\ncss: |\n  body { color: red; }\ntitle: Notes\n---\n# Notes\n",
			css:  "body { color: red; }\n",
			body: "# Notes\n",
		},
		{
			name: "absent",
			text: "# Notes\n\nNo front matter here.\n",
			css:  "",
			body: "# Notes\n\nNo front matter here.\n",
		},
		{
			name: "no css key",
			text: "---\ntitle: Notes\n---\n# Notes\n",
			css:  "",
			body: "# Notes\n",
		},
		{
			name: "non-mapping",
			text: "---\n- one\n- two\n---\n# Notes\n",
			css:  "",
			body: "# Notes\n",
		},
		{
			name: "css is not a string",
			text: "---\ncss:\n  - one\n---\n# Notes\n",
			css:  "",
			body: "# Notes\n",
		},
		{
			name: "malformed",
			text: "---\ncss: \"unterminated\ntitle: [\n---\n# Notes\n",
			css:  "",
			body: "# Notes\n",
		},
		{
			name: "terminated with dots",
			text: "---\ncss: 'p { margin: 0; }'\n...\n# Notes\n",
			css:  "p { margin: 0; }",
			body: "# Notes\n",
		},
		{
			name: "CRLF line endings",
			text: "---\r\ncss: 'p { margin: 0; }'\r\n---\r\n# Notes\r\n",
			css:  "p { margin: 0; }",
			body: "# Notes\r\n",
		},
		{
			name: "empty block",
			text: "---\n---\n# Notes\n",
			css:  "",
			body: "# Notes\n",
		},
		{
			name: "rule rather than front matter",
			text: "Title\n---\nText below the rule.\n",
			css:  "",
			body: "Title\n---\nText below the rule.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			css, _, body := readFrontMatter(test.name+".md", test.text)
			if css != test.css {
				t.Errorf("css = %q, want %q", css, test.css)
			}
			if body != test.body {
				t.Errorf("body = %q, want %q", body, test.body)
			}
		})
	}
}

func TestReadFrontMatterDescribesTheDocument(t *testing.T) {
	tests := map[string]documentInfo{
		"---\ntitle: Notes\n---\n":                             {Title: "Notes"},
		"---\ntitle: Notes\nauthor: Ann\n---\n":                {Title: "Notes", Author: "Ann"},
		"---\ntitle: Notes\ndate: 2024-05-01\n---\n":           {Title: "Notes", Date: "2024-05-01"},
		"---\ntitle: Notes\ndate: 2024-05-01T10:30:00Z\n---\n": {Title: "Notes", Date: "2024-05-01 10:30"},
		"---\ntitle: Notes\ndate: May 1, 2024\n---\n":          {Title: "Notes", Date: "May 1, 2024"},
		"---\ntitle: 1984\nauthor: [Ann, Bob]\n---\n":          {Title: "1984"},
		"---\n- title\n---\n":                                  {},
		"# Notes\n":                                            {},
	}
	for text, want := range tests {
		t.Run(text, func(t *testing.T) {
			if _, got, _ := readFrontMatter("described.md", text); got != want {
				t.Errorf("info = %+v, want %+v", got, want)
			}
		})
	}
}

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
	text := "print('hi')\n"
	css, _, markdown := asMarkdownDocument("script.py", text)
	if css != "" {
		t.Errorf("css = %q, want empty", css)
	}
	want := "# script.py\n\n```python\nprint('hi')\n```\n"
	if markdown != want {
		t.Errorf("markdown = %q, want %q", markdown, want)
	}
}

func TestAsMarkdownDocumentFenceOutgrowsTheFile(t *testing.T) {
	text := "a\n```\nfenced\n```\n````\nlonger\n````\n"
	_, _, markdown := asMarkdownDocument("notes.txt", text)
	if !strings.HasPrefix(markdown, "# notes.txt\n\n`````plaintext\n") {
		t.Errorf("markdown does not open with a five-backtick fence: %q", markdown)
	}
	if !strings.HasSuffix(markdown, "\n`````\n") {
		t.Errorf("markdown does not close with a five-backtick fence: %q", markdown)
	}
}

func TestAsMarkdownDocumentUnknownSuffixIsItsOwnLanguage(t *testing.T) {
	_, _, markdown := asMarkdownDocument("main.go", "package main\n")
	if !strings.Contains(markdown, "```go\n") {
		t.Errorf("markdown = %q, want a go fence", markdown)
	}
}

func TestAsMarkdownDocumentMarkdownKeepsItsText(t *testing.T) {
	_, _, markdown := asMarkdownDocument("notes.MARKDOWN", "# Notes\n")
	if markdown != "# Notes\n" {
		t.Errorf("markdown = %q, want the text unchanged", markdown)
	}
}
