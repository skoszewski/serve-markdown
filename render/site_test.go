package render

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// siteFiles is a document set as an embed.FS holds it: without modification times.
var siteFiles = fstest.MapFS{
	"guide.md":          {Data: []byte("---\ntitle: The Guide\ncss: 'p { margin: 0; }'\n---\n# Guide\n\n[Notes](notes/)\n")},
	"examples.md":       {Data: []byte("# Examples\n")},
	"notes/one.md":      {Data: []byte("# One\n")},
	"notes/two.md":      {Data: []byte("# Two\n")},
	"pictures/logo.png": {Data: []byte("\x89PNG\r\n\x1a\nnot a real picture\n")},
	".hidden.md":        {Data: []byte("# Hidden\n")},
	"script.sh":         {Data: []byte("echo hi\n")},
}

// siteAssets stands for the built files the importing program supplies.
var siteAssets = fstest.MapFS{
	"page.js":        {Data: []byte("// page")},
	"mermaid.min.js": {Data: []byte("// mermaid")},
}

// fetch performs one request against handler and returns the response and its body.
func fetch(t *testing.T, handler http.Handler, target string) (*http.Response, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	response := recorder.Result()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(body)
}

// content fetches the document route addresses through the site's content route.
func content(t *testing.T, site *Site, route string) Content {
	t.Helper()
	_, body := fetch(t, site, "/docs/_/content?path="+route)
	var read Content
	if err := json.Unmarshal([]byte(body), &read); err != nil {
		t.Fatalf("the content of %s does not read as JSON: %v: %q", route, err, body)
	}
	return read
}

func TestSiteServesThePageShellUnderItsPrefix(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/", Assets: siteAssets, Search: IndexSearch{"guide.md"},
		Banner: "Docs 1.0", WatchInterval: 2 * time.Second})

	for _, route := range []string{"/docs/", "/docs", "/docs/guide.md"} {
		response, page := fetch(t, site, route)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", route, response.StatusCode)
		}
		for _, want := range []string{"<title>guide.md</title>", `src="/docs/_/assets/page.js"`,
			`href="/docs/_/page/page.css"`, `"contentURL":"/docs/_/content?path=`, `"watchIntervalMS":2000`,
			`<span id="_banner">Docs 1.0</span>`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the page does not hold %q: %q", route, want, page)
			}
		}
	}
}

func TestSiteAnswersTheDocument(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/", Search: IndexSearch{"guide.md"}, CSS: "body { margin: 0; }"})

	read := content(t, site, "/docs/")
	if read.Error != nil {
		t.Fatalf("error = %q", *read.Error)
	}
	if *read.Text != "# Guide\n\n[Notes](notes/)\n" || *read.CSS != "p { margin: 0; }" ||
		read.Info.Title != "The Guide" || *read.Base != "/docs/" {
		t.Errorf("content = %q %q %+v %q, want the guide without its front matter", *read.Text, *read.CSS,
			*read.Info, *read.Base)
	}
	// A file without modification times is marked by its text, the same on every read.
	if again := content(t, site, "/docs/"); *again.MTime != *read.MTime || *read.MTime == "" {
		t.Errorf("marker = %q, then %q, want one stable marker", *read.MTime, *again.MTime)
	}

	// A document whose front matter declares no CSS takes the site's own.
	if read := content(t, site, "/docs/examples.md"); *read.CSS != "body { margin: 0; }" {
		t.Errorf("css = %q, want the site's", *read.CSS)
	}
	// Another file is shown as a code block.
	if read := content(t, site, "/docs/script.sh"); !strings.Contains(*read.Text, "```bash\necho hi\n```") {
		t.Errorf("text = %q, want a bash code block", *read.Text)
	}
}

func TestSiteListsAFolderWithoutItsDocument(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/"})

	read := content(t, site, "/docs/notes")
	if read.Error != nil {
		t.Fatalf("error = %q", *read.Error)
	}
	if want := "# notes\n\n- [one.md](one.md)\n- [two.md](two.md)\n"; *read.Text != want || *read.Base != "/docs/notes/" {
		t.Errorf("content = %q %q, want %q under /docs/notes/", *read.Text, *read.Base, want)
	}
	if listing := *content(t, site, "/docs/").Text; strings.Contains(listing, ".hidden.md") {
		t.Errorf("the listing names a hidden file: %q", listing)
	}
}

func TestSiteServesPictures(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/"})

	response, body := fetch(t, site, "/docs/pictures/logo.png")
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(body, "\x89PNG") {
		t.Errorf("status = %d, body = %q, want the picture", response.StatusCode, body)
	}
	if policy := response.Header.Get("Content-Security-Policy"); policy != "sandbox" {
		t.Errorf("Content-Security-Policy = %q, want sandbox", policy)
	}
}

func TestSiteRefusesWhatIsNotServed(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/"})

	for _, route := range []string{"/docs/.hidden.md", "/docs/nowhere.md", "/docs/../secret.md", "/other/guide.md",
		"/docs/%2e%2e/secret.md"} {
		if response, _ := fetch(t, site, route); response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", route, response.StatusCode)
		}
		if read := content(t, site, route); read.Error == nil {
			t.Errorf("%s: the content route answered a document", route)
		}
	}
}

func TestSiteServesItsAssets(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/", Assets: siteAssets})

	for _, route := range []string{"/docs/_/assets/page.js", "/docs/_/page/page.css", "/docs/_/page/page-dark.css"} {
		if response, body := fetch(t, site, route); response.StatusCode != http.StatusOK || body == "" {
			t.Errorf("%s: status = %d, want the file", route, response.StatusCode)
		}
	}
	if response, _ := fetch(t, site, "/docs/_/assets/nothing.js"); response.StatusCode != http.StatusNotFound {
		t.Errorf("a missing asset: status = %d, want 404", response.StatusCode)
	}
}

func TestSiteDrawsDiagramsOnlyWithTheBundle(t *testing.T) {
	if _, page := fetch(t, New(siteFiles, Options{Prefix: "/docs/", Assets: siteAssets, Mermaid: true}),
		"/docs/guide.md"); !strings.Contains(page, `"mermaidJS":"/docs/_/assets/mermaid.min.js"`) {
		t.Errorf("the page does not draw diagrams: %q", page)
	}
	withoutBundle := fstest.MapFS{"page.js": {Data: []byte("// page")}}
	if _, page := fetch(t, New(siteFiles, Options{Prefix: "/docs/", Assets: withoutBundle, Mermaid: true}),
		"/docs/guide.md"); !strings.Contains(page, `"mermaidJS":""`) {
		t.Errorf("the page loads a bundle that is not there: %q", page)
	}
}

func TestSiteMenuStandsInPlaceOfTheList(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/", Search: IndexSearch{"guide.md"}, List: DefaultList,
		Menu: []MenuEntry{{Path: "guide.md"}, {Label: "Examples", Path: "examples.md"}, {Path: ".hidden.md"}}})

	_, page := fetch(t, site, "/docs/")
	for _, want := range []string{`<nav id="_list" class="sidebar menu">`,
		`<a href="/docs/guide.md" class="current">The Guide</a>`, `<a href="/docs/examples.md">Examples</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}
	if strings.Contains(page, `>Hidden</a>`) || strings.Contains(page, ".hidden.md") || strings.Contains(page, "one.md") {
		t.Errorf("the menu holds a hidden document, or the list stands beside it: %q", page)
	}
}

func TestSiteListsTheFolder(t *testing.T) {
	site := New(siteFiles, Options{Prefix: "/docs/", List: DefaultList})

	_, page := fetch(t, site, "/docs/notes/one.md")
	for _, want := range []string{`<a href="/docs/notes/one.md" class="current">one.md</a>`,
		`<a href="/docs/notes/two.md">two.md</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}
}
