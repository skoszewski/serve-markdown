package render

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFrontMatter(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		css    string
		body   string
		failed bool
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
			name:   "malformed",
			text:   "---\ncss: \"unterminated\ntitle: [\n---\n# Notes\n",
			css:    "",
			body:   "# Notes\n",
			failed: true,
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
			css, _, body, err := FrontMatter(test.text)
			if css != test.css {
				t.Errorf("css = %q, want %q", css, test.css)
			}
			if body != test.body {
				t.Errorf("body = %q, want %q", body, test.body)
			}
			if (err != nil) != test.failed {
				t.Errorf("err = %v, want an error %v", err, test.failed)
			}
		})
	}
}

func TestFrontMatterDescribesTheDocument(t *testing.T) {
	tests := map[string]DocumentInfo{
		"---\ntitle: Notes\n---\n":                             {Title: "Notes"},
		"---\ntitle: Notes\nauthor: Ann\n---\n":                {Title: "Notes", Author: "Ann"},
		"---\ntitle: Notes\ndate: 2024-05-01\n---\n":           {Title: "Notes", Date: "2024-05-01"},
		"---\ntitle: Notes\ndate: 2024-05-01T10:30:00Z\n---\n": {Title: "Notes", Date: "2024-05-01 10:30"},
		"---\ntitle: Notes\ndate: May 1, 2024\n---\n":          {Title: "Notes", Date: "May 1, 2024"},
		"---\ntitle: 1984\nauthor: [Ann, Bob]\n---\n":          {Title: "1984"},
		"---\n- title\n---\n":                                  {},
		"# Notes\n":                                            {},
		"---\npdf:\n  margin: 25mm\n  header: title\n---\n": {
			PDF: map[string]string{"margin": "25mm", "header": "title"}},
		"---\npdf: a4\n---\n": {},
	}
	for text, want := range tests {
		t.Run(text, func(t *testing.T) {
			if _, got, _, _ := FrontMatter(text); !reflect.DeepEqual(got, want) {
				t.Errorf("info = %+v, want %+v", got, want)
			}
		})
	}
}

func TestDocumentInfoKeepsThePDFPageOnTheServer(t *testing.T) {
	sent, err := json.Marshal(DocumentInfo{Title: "Notes", PDF: map[string]string{"margin": "25mm"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sent), "margin") {
		t.Errorf("the page is sent the PDF page: %s", sent)
	}
}

func TestCodeDocumentWrapsTheFile(t *testing.T) {
	want := "# script.py\n\n```python\nprint('hi')\n```\n"
	if markdown := CodeDocument("script.py", "print('hi')\n"); markdown != want {
		t.Errorf("markdown = %q, want %q", markdown, want)
	}
}

func TestCodeDocumentFenceOutgrowsTheFile(t *testing.T) {
	markdown := CodeDocument("notes.txt", "a\n```\nfenced\n```\n````\nlonger\n````\n")
	if !strings.HasPrefix(markdown, "# notes.txt\n\n`````plaintext\n") {
		t.Errorf("markdown does not open with a five-backtick fence: %q", markdown)
	}
	if !strings.HasSuffix(markdown, "\n`````\n") {
		t.Errorf("markdown does not close with a five-backtick fence: %q", markdown)
	}
}

func TestCodeDocumentUnknownSuffixIsItsOwnLanguage(t *testing.T) {
	if markdown := CodeDocument("main.go", "package main\n"); !strings.Contains(markdown, "```go\n") {
		t.Errorf("markdown = %q, want a go fence", markdown)
	}
}

func TestDocumentBase(t *testing.T) {
	tests := []struct {
		route string
		name  string
		want  string
	}{
		// A route ending in the document's own name reads its links from the folder above it.
		{"/_/ado/org/project/repo/pool/README.md", "README.md", "/_/ado/org/project/repo/pool/"},
		{"/_/ado/org/project/repo/README.md", "README.md", "/_/ado/org/project/repo/"},
		{"/docs/guide.md", "guide.md", "/docs/"},
		// A route naming a folder resolved to a document within it, however it was written.
		{"/_/ado/org/project/repo", "README.md", "/_/ado/org/project/repo/"},
		{"/_/ado/org/project/repo/", "README.md", "/_/ado/org/project/repo/"},
		{"/_/ado/org/project/repo/subdir", "index.md", "/_/ado/org/project/repo/subdir/"},
		{"/docs", "index.md", "/docs/"},
		{"/", "README.md", "/"},
		{"", "README.md", "/"},
		// An escaped route names the same document as the name it was read from.
		{"/docs/my%20guide.md", "my guide.md", "/docs/"},
		// A listing the server wrote has no file of its own.
		{"/docs", "", "/docs/"},
	}

	for _, test := range tests {
		t.Run(test.route, func(t *testing.T) {
			if got := DocumentBase(test.route, test.name); got != test.want {
				t.Errorf("DocumentBase(%q, %q) = %q, want %q", test.route, test.name, got, test.want)
			}
		})
	}
}
