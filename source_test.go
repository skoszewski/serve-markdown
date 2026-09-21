package main

import (
	"os"
	"path/filepath"
	"strings"
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
		"listing/one.md":   "# One\n",
		"listing/two.md":   "# Two\n",
		// A directory holding both documents, for the order they are looked for in.
		"both/index.md":  "# Both index\n",
		"both/README.md": "# Both readme\n",
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
		{"the root is the server's directory", "/", "."},
		{"a file below the root", "/notes.md", "notes.md"},
		{"a subdirectory is itself", "/docs", "docs"},
		{"a subdirectory with a trailing slash", "/docs/", "docs"},
		{"a file below a subdirectory", "/docs/guide.md", "docs/guide.md"},
		{"a percent-escaped route", "/docs/%67uide.md", "docs/guide.md"},
		{"a traversal attempt", "/../../etc/passwd", ""},
		{"a traversal attempt below a subdirectory", "/docs/../../secrets.md", ""},
		{"a missing file", "/nowhere.md", ""},
		{"a directory holding no document", "/empty", "empty"},
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

func TestRenderDirectoryListing(t *testing.T) {
	root := documentRoot(t)
	names, err := markdownFilesIn(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "# " + filepath.Base(root) + "\n\n" +
		"- [Alpha.md](Alpha.md)\n" +
		"- [beta.markdown](beta.markdown)\n" +
		"- [notes.md](notes.md)\n" +
		"- [README.md](README.md)\n"
	if listing := renderDirectoryListing(root, names); listing != want {
		t.Errorf("listing = %q, want %q", listing, want)
	}

	// --index-only asks for the listing with no names, having stopped looking for them.
	if listing := renderDirectoryListing(root, nil); listing != "# "+filepath.Base(root)+"\n\nNo Markdown files found.\n" {
		t.Errorf("listing = %q, want the one saying none were found", listing)
	}
}

func TestRenderDirectoryListingWithoutDocuments(t *testing.T) {
	root := documentRoot(t)
	empty := filepath.Join(root, "empty")
	names, err := markdownFilesIn(empty)
	if err != nil {
		t.Fatal(err)
	}
	if listing := renderDirectoryListing(empty, names); listing != "# empty\n\nNo Markdown files found.\n" {
		t.Errorf("listing = %q, want the one saying none were found", listing)
	}
}

func TestResolveSource(t *testing.T) {
	localSource := source{kind: kindLocal}
	adoSource := source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/README.md"}

	tests := []struct {
		name          string
		route         string
		defaultSource source
		want          source
		ok            bool
	}{
		{"a document", "/notes.md", localSource,
			source{kind: kindLocal, route: "/notes.md"}, true},
		{"a directory", "/docs", localSource,
			source{kind: kindLocal, route: "/docs"}, true},
		{"the root", "/", localSource,
			source{kind: kindLocal, route: "/"}, true},
		{"the ado namespace", "/_/ado/org/proj/repo/docs/guide.md", localSource,
			source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}, true},
		{"an ado project", "/_/ado/org/proj", localSource,
			source{kind: kindADO, organization: "org", project: "proj"}, true},
		{"an ado organization", "/_/ado/org", localSource,
			source{kind: kindADO, organization: "org"}, true},
		{"the ado namespace naming nothing", "/_/ado/", localSource, source{}, false},
		{"an unknown namespace", "/_/elsewhere/module", localSource, source{}, false},
		// Every route outside the namespace is local, whatever the server was started with, so
		// an ado:// source keeps its own route and the rest of the server stays local.
		{"a plain route under an ado source", "/docs/guide.md", adoSource,
			source{kind: kindLocal, route: "/docs/guide.md"}, true},
		{"the root under an ado source", "/", adoSource,
			source{kind: kindLocal, route: "/"}, true},
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

func TestLoadDocumentReadsADirectory(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}

	tests := map[string]struct {
		name string
		text string
	}{
		// A directory is the document it holds, and the listing of its documents when it holds
		// neither index.md nor README.md. A local directory names its index first.
		"/docs":    {"index.md", "# Docs\n"},
		"/plain":   {"README.md", "# Plain\n"},
		"/both":    {"index.md", "# Both index\n"},
		"/listing": {"", "# listing\n\n- [one.md](one.md)\n- [two.md](two.md)\n"},
		"/empty":   {"", "# empty\n\nNo Markdown files found.\n"},
	}
	for route, want := range tests {
		t.Run(route, func(t *testing.T) {
			read, err := handler.loadDocument(source{kind: kindLocal, route: route})
			if err != nil {
				t.Fatal(err)
			}
			if read.marker == "" {
				t.Error("the document carries no change marker")
			}
			if read.name != want.name {
				t.Errorf("name = %q, want %q", read.name, want.name)
			}
			if read.text != want.text {
				t.Errorf("text = %q, want %q", read.text, want.text)
			}
		})
	}
}

func TestLoadDocumentReadsOnlyTheIndex(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, indexOnly: true}

	tests := map[string]struct {
		name string
		text string
	}{
		// The index is still read; a directory holding none is answered as holding no Markdown
		// at all, whatever else stands in it.
		"/docs":    {"index.md", "# Docs\n"},
		"/plain":   {"README.md", "# Plain\n"},
		"/listing": {"", "# listing\n\nNo Markdown files found.\n"},
		"/empty":   {"", "# empty\n\nNo Markdown files found.\n"},
	}
	for route, want := range tests {
		t.Run(route, func(t *testing.T) {
			read, err := handler.loadDocument(source{kind: kindLocal, route: route})
			if err != nil {
				t.Fatal(err)
			}
			if read.name != want.name || read.text != want.text {
				t.Errorf("document = %q, %q, want %q, %q", read.name, read.text, want.name, want.text)
			}
		})
	}

	// The document beside the index is still served when the route names it.
	read, err := handler.loadDocument(source{kind: kindLocal, route: "/listing/one.md"})
	if err != nil {
		t.Fatal(err)
	}
	if read.text != "# One\n" {
		t.Errorf("text = %q, want the document the route names", read.text)
	}

	// The list beside the page still reaches every document, the flag being the page's own.
	entries, _ := handler.documentList(source{kind: kindLocal, route: "/listing"}, "current")
	if len(entries) != 2 {
		t.Errorf("the list holds %+v, want both documents of the folder", entries)
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
			if got := documentBase(test.route, test.name); got != test.want {
				t.Errorf("documentBase(%q, %q) = %q, want %q", test.route, test.name, got, test.want)
			}
		})
	}
}

func TestLoadDocumentReportsAMissingFile(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}
	if _, err := handler.loadDocument(source{kind: kindLocal, route: "/nowhere.md"}); err == nil {
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

func TestDocumentTreeListsALocalDirectory(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}
	src := source{kind: kindLocal, route: "/docs/guide.md"}

	names := func(entries []listEntry) []string {
		var read []string
		for _, entry := range entries {
			read = append(read, entry.Name)
			for _, child := range entry.Children {
				read = append(read, entry.Name+"/"+child.Name)
			}
		}
		return read
	}

	tests := map[string][]string{
		"current":    {"guide.md", "index.md"},
		"subfolders": {"..", "guide.md", "index.md"},
		"tree": {"both", "both/index.md", "both/README.md", "docs", "docs/guide.md", "docs/index.md",
			"listing", "listing/one.md", "listing/two.md", "plain", "plain/README.md",
			"Alpha.md", "beta.markdown", "notes.md", "README.md"},
	}
	for scope, want := range tests {
		t.Run(scope, func(t *testing.T) {
			entries, _ := handler.documentList(src, scope)
			got := names(entries)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("entries = %v, want %v", got, want)
			}
		})
	}
}

func TestDocumentTreeRoutesAndMarksTheCurrentDocument(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}

	entries, up := handler.documentList(source{kind: kindLocal, route: "/docs/guide.md"}, "subfolders")
	if up.Label != "" {
		t.Errorf("a local list carries the link %q above it", up.Label)
	}
	for _, entry := range entries {
		if entry.Name == "guide.md" {
			if entry.Route != "/docs/guide.md" {
				t.Errorf("route = %q, want \"/docs/guide.md\"", entry.Route)
			}
			if !entry.Current {
				t.Error("the document the page shows is not marked")
			}
		}
		if entry.Name == ".." && entry.Route != "/" {
			t.Errorf("the parent route = %q, want \"/\"", entry.Route)
		}
		if entry.Name == "index.md" && entry.Current {
			t.Error("another document is marked as the one the page shows")
		}
	}
}

func TestDocumentListIgnoresARouteItCannotRead(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}

	for _, route := range []string{"/nowhere.md", "/../.."} {
		t.Run(route, func(t *testing.T) {
			if entries, _ := handler.documentList(source{kind: kindLocal, route: route}, "tree"); entries != nil {
				t.Errorf("entries = %v, want none", entries)
			}
		})
	}
}

func TestResolveRouteRefusesClimbingOutOfTheRoot(t *testing.T) {
	root := documentRoot(t)
	// The directory the server was started in is a subdirectory, as it is when a file was
	// named on the command line.
	started := filepath.Join(root, "docs")
	writeFile(t, filepath.Join(root, "secret.md"), "# Secret\n")

	inside := map[string]string{
		"/guide.md":             "docs/guide.md",
		"/../docs/guide.md":     "docs/guide.md",
		"/subdir/../index.md":   "docs/index.md",
		"/./index.md":           "docs/index.md",
		"/%2e%2e/docs/index.md": "docs/index.md",
	}
	for route, want := range inside {
		t.Run("inside "+route, func(t *testing.T) {
			got := resolveRoute(route, started, "")
			if got != filepath.Join(root, filepath.FromSlash(want)) {
				t.Errorf("resolveRoute(%q) = %q, want the document itself", route, got)
			}
		})
	}

	outside := []string{
		"/../secret.md",
		"/../../secret.md",
		"/%2e%2e/secret.md",
		"/..%2f..%2fsecret.md",
		"/subdir/../../secret.md",
		"/../",
		"/..",
	}
	for _, route := range outside {
		t.Run("outside "+route, func(t *testing.T) {
			if got := resolveRoute(route, started, ""); got != "" {
				t.Errorf("resolveRoute(%q) = %q, want nothing outside the root", route, got)
			}
		})
	}
}

func TestDocumentTreeStaysInsideTheRoot(t *testing.T) {
	root := documentRoot(t)
	started := filepath.Join(root, "docs")
	writeFile(t, filepath.Join(root, "secret.md"), "# Secret\n")
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: started}

	for _, route := range []string{"/", "/guide.md", "/../", "/../secret.md", "/%2e%2e"} {
		t.Run(route, func(t *testing.T) {
			tree, _ := handler.documentList(source{kind: kindLocal, route: route}, "tree")
			for _, entry := range tree {
				if strings.Contains(entry.Name, "secret") || strings.Contains(entry.Route, "secret") {
					t.Errorf("the list names %q, outside the root", entry.Route)
				}
			}
			// The parent of the root is the root itself, so '..' never leads out of it.
			below, _ := handler.documentList(source{kind: kindLocal, route: route}, "subfolders")
			for _, entry := range below {
				if entry.Name == ".." {
					t.Errorf("the list offers a way above the root: %q", entry.Route)
				}
			}
		})
	}
}
