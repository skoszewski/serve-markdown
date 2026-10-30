package main

import (
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// ignoring returns the rules of the patterns given.
func ignoring(t *testing.T, patterns ...string) *ignoreRules {
	t.Helper()
	rules, err := readIgnoreRules("", patterns)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestIgnoreRulesFollowGitignore(t *testing.T) {
	rules := ignoring(t, "# a comment", "", "*.secret.md", "!keep.secret.md", "/top.md", "docs/old",
		"build/", "**/cache", "notes/**")
	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"a.secret.md", false, true},
		{"docs/a.secret.md", false, true},
		{"docs/keep.secret.md", false, false},
		{"top.md", false, true},
		{"docs/top.md", false, false},
		{"docs/old", true, true},
		{"docs/old/a.md", false, true},
		{"x/docs/old/a.md", false, false},
		{"build", true, true},
		{"build", false, false},
		{"docs/build/a.md", false, true},
		{"a/b/cache/c.md", false, true},
		{"notes/a/b.md", false, true},
		{"notes", true, false},
		{"guide.md", false, false},
		{"", true, false},
	}
	for _, test := range tests {
		if got := rules.ignored(test.path, test.isDir); got != test.want {
			t.Errorf("ignored(%q, %v) = %v, want %v", test.path, test.isDir, got, test.want)
		}
	}
}

func TestIgnoreRulesAddTheExtraPatternsAfterTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), ignoreFileName)
	writeFile(t, file, "drafts/\n*.tmp.md\n")
	rules, err := readIgnoreRules(file, []string{"!keep.tmp.md"}, []string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	for relative, want := range map[string]bool{
		"drafts/a.md":   true,
		"a.tmp.md":      true,
		"keep.tmp.md":   false,
		"old/a.md":      true,
		"docs/guide.md": false,
	} {
		if got := rules.ignored(relative, false); got != want {
			t.Errorf("ignored(%q) = %v, want %v", relative, got, want)
		}
	}
}

func TestIgnoreRulesRefuseWhatTheyCannotRead(t *testing.T) {
	directory := t.TempDir()
	if _, err := readIgnoreRules(filepath.Join(directory, ignoreFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: error = %v, want one wrapping fs.ErrNotExist", err)
	}
	file := filepath.Join(directory, "bad")
	writeFile(t, file, "fine.md\n[a\n")
	if _, err := readIgnoreRules(file); err == nil || !strings.Contains(err.Error(), file+":2:") {
		t.Errorf("a bad pattern in a file: error = %v, want one naming its line", err)
	}
	if _, err := readIgnoreRules("", []string{"[a"}); err == nil {
		t.Error("a bad extra pattern: no error")
	}
	var none *ignoreRules
	if none.ignored("a.md", false) || none.ignoredPath(filepath.Join(directory, "a.md"), false) {
		t.Error("nil rules ignore a path")
	}
}

func TestServerNeitherListsNorServesIgnoredPaths(t *testing.T) {
	root := documentRoot(t)
	writeFile(t, filepath.Join(root, "drafts", "draft.md"), "# Draft\n")
	writeFile(t, filepath.Join(root, "plan.secret.md"), "# Secret\n")
	writeFile(t, filepath.Join(root, "docs", "hidden.png"), pictureContent)
	rules := ignoring(t, "drafts/", "*.secret.md", "hidden.png", "plain/README.md")
	rules.rootDir = root
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, ignore: rules}

	for _, route := range []string{"/drafts", "/drafts/draft.md", "/plan.secret.md", "/docs/hidden.png",
		"/docs/../plan.secret.md", "/content?path=/drafts/draft.md"} {
		response, body := get(t, handler, route)
		if strings.HasPrefix(route, "/content") {
			if !strings.Contains(body, `"error":`) || strings.Contains(body, "# Draft") {
				t.Errorf("%s: body = %q, want an error", route, body)
			}
			continue
		}
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", route, response.StatusCode)
		}
	}
	if response, _ := get(t, handler, "/docs/guide.md"); response.StatusCode != http.StatusOK {
		t.Errorf("a page not ignored: status = %d, want 200", response.StatusCode)
	}

	tree, _ := handler.documentList(source{kind: kindLocal, route: "/"}, "tree")
	for _, entry := range tree {
		if strings.Contains(entry.Route, "drafts") || strings.Contains(entry.Name, "secret") {
			t.Errorf("the list names %q, which is ignored", entry.Route)
		}
	}
	names, err := markdownFilesIn(root, rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.Contains(name, "secret") {
			t.Errorf("the listing names %q, which is ignored", name)
		}
	}

	// An ignored README.md is not read as its folder's document, which is listed instead.
	document, err := handler.loadDocument(source{kind: kindLocal, route: "/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if document.name != "" || strings.Contains(document.text, "README.md") {
		t.Errorf("the folder's document = %q %q, want the listing without README.md", document.name, document.text)
	}
}
