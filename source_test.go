package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The bytes of the files served as raw assets rather than as documents.
const (
	pictureContent = "\x89PNG\r\n\x1a\nnot a real picture\n"
	drawingContent = "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>\n"
)

// documentRoot builds a directory tree the route tests resolve against.
func documentRoot(t *testing.T) string {
	t.Helper()
	// The temporary directory is resolved here, since routes resolve through symbolic links.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"README.md":        "# Root\n",
		"notes.md":         "# Notes\n",
		"Alpha.md":         "# Alpha\n",
		"beta.markdown":    "# Beta\n",
		"picture.png":      pictureContent,
		"docs/drawing.svg": drawingContent,
		"docs/index.md":    "# Docs\n",
		"docs/guide.md":    "# Guide\n",
		"plain/README.md":  "# Plain\n",
	}
	for name, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveRoute(t *testing.T) {
	root := documentRoot(t)
	tests := []struct {
		name  string
		route string
		want  string
	}{
		{"root resolves to the index file", "/", "README.md"},
		{"a file below the root", "/notes.md", "notes.md"},
		{"a subdirectory resolves to its index file", "/docs", "docs/index.md"},
		{"a subdirectory with a trailing slash", "/docs/", "docs/index.md"},
		{"a file below a subdirectory", "/docs/guide.md", "docs/guide.md"},
		{"a percent-escaped route", "/docs/%67uide.md", "docs/guide.md"},
		{"a traversal attempt", "/../../etc/passwd", ""},
		{"a traversal attempt below a subdirectory", "/docs/../../secrets.md", ""},
		{"a missing file", "/nowhere.md", ""},
		{"a directory without an index file", "/empty", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveRoute(test.route, root, "")
			want := ""
			if test.want != "" {
				want = filepath.Join(root, filepath.FromSlash(test.want))
			}
			if got != want {
				t.Errorf("resolveRoute(%q) = %q, want %q", test.route, got, want)
			}
		})
	}
}

func TestResolveRouteServesTheDefaultFileAtTheRoot(t *testing.T) {
	root := documentRoot(t)
	defaultFile := filepath.Join(root, "notes.md")
	if got := resolveRoute("/", root, defaultFile); got != defaultFile {
		t.Errorf("resolveRoute(\"/\") = %q, want %q", got, defaultFile)
	}
}

func TestResolveDirRoute(t *testing.T) {
	root := documentRoot(t)
	tests := []struct {
		route string
		want  string
	}{
		{"/", "."},
		{"/notes.md", "notes.md"},
		{"/docs", "docs"},
		{"/empty", "empty"},
		{"/nowhere.md", ""},
		{"/../..", ""},
	}

	for _, test := range tests {
		t.Run(test.route, func(t *testing.T) {
			got := resolveDirRoute(test.route, root)
			want := ""
			if test.want != "" {
				want = filepath.Join(root, filepath.FromSlash(test.want))
			}
			if got != want {
				t.Errorf("resolveDirRoute(%q) = %q, want %q", test.route, got, want)
			}
		})
	}
}

func TestRenderDirectoryListing(t *testing.T) {
	root := documentRoot(t)
	listing, err := renderDirectoryListing(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "# " + filepath.Base(root) + "\n\n" +
		"- [Alpha.md](Alpha.md)\n" +
		"- [beta.markdown](beta.markdown)\n" +
		"- [notes.md](notes.md)\n" +
		"- [README.md](README.md)\n"
	if listing != want {
		t.Errorf("listing = %q, want %q", listing, want)
	}
}

func TestResolveSource(t *testing.T) {
	fileSource := source{kind: kindFile}
	adoSource := source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/README.md"}

	tests := []struct {
		name          string
		route         string
		defaultSource source
		want          source
		ok            bool
	}{
		{"a plain route reads the default file source", "/notes.md", fileSource,
			source{kind: kindFile, route: "/notes.md"}, true},
		{"a plain route reads the default dir source", "/notes.md", source{kind: kindDir},
			source{kind: kindDir, route: "/notes.md"}, true},
		{"the file namespace", "/_/file/docs/guide.md", source{kind: kindDir},
			source{kind: kindFile, route: "/docs/guide.md"}, true},
		{"the dir namespace", "/_/dir/docs", fileSource,
			source{kind: kindDir, route: "/docs"}, true},
		{"the ado namespace", "/_/ado/org/proj/repo/docs/guide.md", fileSource,
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}, true},
		{"an incomplete ado route", "/_/ado/org/proj", fileSource, source{}, false},
		{"an unknown namespace", "/_/pydoc/module", fileSource, source{}, false},
		{"the root of an ado source", "/", adoSource, adoSource, true},
		{"a path below an ado source", "/docs/guide.md", adoSource,
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}, true},
		{"a path beside an ado document", "/diagram.png",
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"},
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/diagram.png"}, true},
		{"a path below an ado folder", "/guide.md",
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/"},
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := resolveSource(test.route, test.defaultSource)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Errorf("source = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestLoadDocumentListsADirectory(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindDir}, rootDir: root}
	marker, text, css, err := handler.loadDocument(source{kind: kindDir, route: "/docs"})
	if err != nil {
		t.Fatal(err)
	}
	if marker == "" {
		t.Error("the listing carries no change marker")
	}
	if css != "" {
		t.Errorf("css = %q, want empty", css)
	}
	want := "# docs\n\n- [guide.md](guide.md)\n- [index.md](index.md)\n"
	if text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
}

func TestLoadDocumentReportsAMissingFile(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root}
	if _, _, _, err := handler.loadDocument(source{kind: kindFile, route: "/nowhere.md"}); err == nil {
		t.Fatal("reading a missing file returned no error")
	}
}

// writeFile writes content to path, creating the directories above it.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
