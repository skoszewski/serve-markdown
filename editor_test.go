package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// editorRoot returns a directory of documents with the server editing it.
func editorRoot(t *testing.T) (string, *server) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"README.md":                "# Root\n",
		"docs/guide.md":            "---\n# The guide's own keys.\ntitle: Guide\ncss: |\n  .markdown-body { color: red; }\ntags: [one, two]\n---\n\n# Guide\n",
		"docs/picture.svg":         "<svg/>",
		"reference/generated.md":   "# Generated\n",
		".hidden/secret.md":        "# Secret\n",
		"templates/howto.md":       "---\ntitle: How-to\ndescription: Steps to do one thing.\n---\n\n## Steps\n",
		"docs/drafts/draft.md":     "# Draft\n",
		"docs/drafts/old.markdown": "# Old\n",
		"docs/drafts/notes.txt":    "notes",
		"docs/drafts/.dot.md":      "# Dot\n",
	}
	for name, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
	}
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		editor: &editor{rootDir: root, templatesDir: filepath.Join(root, "templates"),
			exclude: []string{"reference", "templates", "docs/drafts/old.*"}}}
	return root, handler
}

// editorRequest performs one request with a JSON body against handler, empty for none, and
// returns the response and its body.
func editorRequest(t *testing.T, handler *server, method, target, body string) (*http.Response, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(content)
}

func TestEditorIsNotFoundWithoutTheFlag(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1}

	for _, target := range []string{"/_/editor/", "/_/editor/api/files"} {
		if response, _ := get(t, handler, target); response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, response.StatusCode)
		}
	}
}

func TestEditorServesItsShell(t *testing.T) {
	_, handler := editorRoot(t)

	response, body := get(t, handler, "/_/editor/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	for _, want := range []string{assetRoute + "editor.js", assetRoute + "editor.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("the shell does not hold %q", want)
		}
	}

	response, _ = get(t, handler, "/_/editor")
	if response.StatusCode != http.StatusMovedPermanently || response.Header.Get("Location") != editorRoute {
		t.Errorf("status = %d, location = %q, want a redirect to %s", response.StatusCode,
			response.Header.Get("Location"), editorRoute)
	}
}

func TestEditorListsTheEditableFiles(t *testing.T) {
	_, handler := editorRoot(t)

	_, body := get(t, handler, "/_/editor/api/files")
	var files []string
	if err := json.Unmarshal([]byte(body), &files); err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "docs/drafts/draft.md", "docs/guide.md"}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files = %q, want %q", files, want)
	}
}

func TestEditorReadsTheProperties(t *testing.T) {
	_, handler := editorRoot(t)

	_, body := get(t, handler, "/_/editor/api/file?path=docs/guide.md")
	var document struct {
		Properties editorProperties `json:"properties"`
		Body       string           `json:"body"`
	}
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		t.Fatal(err)
	}
	if want := (editorProperties{Title: "Guide", Tags: []string{"one", "two"}}); !reflect.DeepEqual(document.Properties, want) {
		t.Errorf("properties = %+v, want %+v", document.Properties, want)
	}
	if document.Body != "# Guide\n" {
		t.Errorf("body = %q", document.Body)
	}

	if response, _ := get(t, handler, "/_/editor/api/file?path=reference/generated.md"); response.StatusCode != http.StatusNotFound {
		t.Errorf("an excluded file: status = %d, want 404", response.StatusCode)
	}
}

func TestEditorSavesAndKeepsTheOtherKeys(t *testing.T) {
	root, handler := editorRoot(t)

	response, _ := editorRequest(t, handler, http.MethodPut, "/_/editor/api/file?path=docs/guide.md",
		`{"properties":{"title":"The guide","author":"","tags":[]},"body":"\n\n# The guide\r\n\nText"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	content, err := os.ReadFile(filepath.Join(root, "docs", "guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"---\n# The guide's own keys.\ntitle: The guide\n", "css: |\n  .markdown-body { color: red; }\n",
		"updated: 20", "---\n\n# The guide\n\nText\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("the document does not hold %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "tags") || strings.Contains(text, "author") {
		t.Errorf("the empty properties were not removed:\n%s", text)
	}
}

func TestEditorCreatesFiles(t *testing.T) {
	root, handler := editorRoot(t)

	response, _ := editorRequest(t, handler, http.MethodPost, "/_/editor/api/file?path=docs/new-topic/index.md", "{}")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.StatusCode)
	}
	created, frontMatter := readCreated(t, filepath.Join(root, "docs", "new-topic", "index.md"))
	if created.Title != "new topic" || created.Created == "" || created.Created != created.Updated || created.Author != "" {
		t.Errorf("properties = %+v", created)
	}
	if !strings.HasSuffix(frontMatter, "---\n\n") {
		t.Errorf("the document is %q", frontMatter)
	}

	response, _ = editorRequest(t, handler, http.MethodPost, "/_/editor/api/file?path=docs/how_to.md", `{"template":"howto.md"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.StatusCode)
	}
	created, text := readCreated(t, filepath.Join(root, "docs", "how_to.md"))
	if created.Title != "how to" || !strings.HasSuffix(text, "\n## Steps\n") {
		t.Errorf("properties = %+v, document = %q", created, text)
	}

	for target, status := range map[string]int{
		"/_/editor/api/file?path=docs/guide.md":     http.StatusConflict,
		"/_/editor/api/file?path=docs/new.flow.svg": http.StatusBadRequest,
		"/_/editor/api/file?path=../outside.md":     http.StatusBadRequest,
		"/_/editor/api/file?path=docs/.hidden.md":   http.StatusBadRequest,
		"/_/editor/api/file?path=docs/page.html":    http.StatusBadRequest,
		"/_/editor/api/file?path=reference/new.md":  http.StatusBadRequest,
		"/_/editor/api/file?path=/docs/absolute.md": http.StatusBadRequest,
	} {
		if response, body := editorRequest(t, handler, http.MethodPost, target, "{}"); response.StatusCode != status {
			t.Errorf("%s: status = %d, want %d (%s)", target, response.StatusCode, status, body)
		}
	}
	if response, _ := editorRequest(t, handler, http.MethodPost, "/_/editor/api/file?path=docs/y.md", `{"template":"none.md"}`); response.StatusCode != http.StatusBadRequest {
		t.Errorf("an unknown template: status = %d, want 400", response.StatusCode)
	}
}

// readCreated returns the properties and the content of a document the editor created.
func readCreated(t *testing.T, file string) (editorProperties, string) {
	t.Helper()
	frontMatter, _, err := readEditorDocument(file)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return readProperties(frontMatter), string(content)
}

func TestEditorRefusesRequestsFromOtherOrigins(t *testing.T) {
	_, handler := editorRoot(t)

	request := httptest.NewRequest(http.MethodPost, "/_/editor/api/file?path=docs/other.md", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://elsewhere.test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("another origin: status = %d, want 403", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/_/editor/api/file?path=docs/other.md", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "text/plain")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a body that is not JSON: status = %d, want 415", recorder.Code)
	}
}

func TestEditorExcludesByNameAndPath(t *testing.T) {
	e := &editor{exclude: []string{"build", "docs/reference/", "*.draft.md"}}
	for relative, want := range map[string]bool{
		"build/a.md":                 true,
		"docs/build/a.md":            true,
		"docs/reference/a.md":        true,
		"reference/a.md":             false,
		"docs/notes.draft.md":        true,
		"docs/notes.md":              false,
		"docs/reference-guide/a.md":  false,
		"other/docs/reference/a.md":  false,
		"docs/reference/deeper/b.md": true,
	} {
		if got := e.excluded(relative); got != want {
			t.Errorf("excluded(%q) = %v, want %v", relative, got, want)
		}
	}
}

func TestEditorListsTheChangedFilesOfItsDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repository, "site")
	writeFile(t, filepath.Join(root, "README.md"), "# Root\n")
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")
	writeFile(t, filepath.Join(repository, "outside.md"), "# Outside\n")
	e := &editor{rootDir: root}
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch", "main", repository},
		{"-C", repository, "add", "."},
		{"-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "--message", "Start"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide, changed\n")
	writeFile(t, filepath.Join(root, "new.md"), "# New\n")
	writeFile(t, filepath.Join(repository, "outside.md"), "# Outside, changed\n")

	changes, err := e.changedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/guide.md", "new.md"}; !reflect.DeepEqual(changes, want) {
		t.Errorf("changes = %q, want %q", changes, want)
	}
	branches, err := e.listBranches()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"main"}; !reflect.DeepEqual(branches, want) {
		t.Errorf("branches = %q, want %q", branches, want)
	}
}

func TestEditorReadsItsConfigurationFile(t *testing.T) {
	root, handler := editorRoot(t)
	writeFile(t, filepath.Join(root, editorConfigName), `{"directories": ["docs/"], "exclude": ["drafts"]}`)

	config, err := readEditorConfig(filepath.Join(root, editorConfigName))
	if err != nil {
		t.Fatal(err)
	}
	handler.editor.directories, handler.editor.exclude = config.Directories, config.Exclude

	_, body := get(t, handler, "/_/editor/api/files")
	var files []string
	if err := json.Unmarshal([]byte(body), &files); err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/guide.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("files = %q, want %q", files, want)
	}
	if response, _ := editorRequest(t, handler, http.MethodPost, "/_/editor/api/file?path=elsewhere.md", "{}"); response.StatusCode != http.StatusBadRequest {
		t.Errorf("a document outside the directories: status = %d, want 400", response.StatusCode)
	}
}

func TestEditorRefusesABadConfigurationFile(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":    `{`,
		"unknown key": `{"exclude": [], "folders": []}`,
		"outside":     `{"directories": ["../other"]}`,
		"absolute":    `{"directories": ["/etc"]}`,
		"bad pattern": `{"exclude": ["[a"]}`,
		"wrong kind":  `{"exclude": "tmp"}`,
	} {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, editorConfigName), content)
		if _, err := readEditorConfig(filepath.Join(root, editorConfigName)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := readEditorConfig(filepath.Join(t.TempDir(), editorConfigName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no file: error = %v, want one wrapping fs.ErrNotExist", err)
	}
}
