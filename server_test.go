package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// get performs one request against handler and returns the response and its body.
func get(t *testing.T, handler *server, target string) (*http.Response, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	response := recorder.Result()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response, string(body)
}

func TestServeRootReturnsThePageShell(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	response, body := get(t, handler, "/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", contentType)
	}
	for _, want := range []string{"<title>README.md</title>", embeddedAssets.MarkedJS, `"?path=%2F"`, `"watchIntervalMS":1000`, pageAssetRoute + "page.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not hold %q", want)
		}
	}
}

func TestServeContentReturnsTheDocument(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	response, body := get(t, handler, "/content?path=/docs/guide.md")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", contentType)
	}

	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != nil {
		t.Fatalf("error = %q, want none", *payload.Error)
	}
	if payload.MTime == nil || *payload.MTime == "" {
		t.Error("the payload carries no change marker")
	}
	if payload.Text == nil || *payload.Text != "# Guide\n" {
		t.Errorf("text = %v, want \"# Guide\\n\"", payload.Text)
	}
	if payload.CSS == nil || *payload.CSS != "" {
		t.Errorf("css = %v, want empty", payload.CSS)
	}
}

func TestServeContentReportsAMissingDocument(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	_, body := get(t, handler, "/content?path=/nowhere.md")
	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MTime != nil || payload.Text != nil || payload.CSS != nil {
		t.Errorf("payload = %+v, want nulls", payload)
	}
	if payload.Error == nil || !strings.Contains(*payload.Error, "/nowhere.md") {
		t.Errorf("error = %v, want one naming the route", payload.Error)
	}
}

func TestServeContentCarriesTheFrontMatterCSS(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1, assets: embeddedAssets}
	writeFile(t, filepath.Join(root, "styled.md"), "---\ncss: 'p { margin: 0; }'\n---\n# Styled\n")

	_, body := get(t, handler, "/content?path=/styled.md")
	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CSS == nil || *payload.CSS != "p { margin: 0; }" {
		t.Errorf("css = %v, want the declared rule", payload.CSS)
	}
	if payload.Text == nil || *payload.Text != "# Styled\n" {
		t.Errorf("text = %v, want the body without its front matter", payload.Text)
	}
}

func TestServeUnknownRouteReturnsTheRouteHelp(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	response, body := get(t, handler, "/nowhere.md")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
	if !strings.Contains(body, "no document found for '/nowhere.md'") {
		t.Errorf("body does not name the route: %q", body)
	}
	if !strings.Contains(body, routeHelp) {
		t.Errorf("body does not hold the route help: %q", body)
	}
	if strings.Contains(body, "pydoc") {
		t.Errorf("the route help still names pydoc: %q", body)
	}
}

func TestServeEmbeddedAssetsByDefault(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	_, page := get(t, handler, "/")
	for _, host := range []string{"cdn.jsdelivr.net", "cdnjs.cloudflare.com", "http://", "https://"} {
		if strings.Contains(page, host) {
			t.Errorf("the default page references %q", host)
		}
	}

	for _, name := range []string{"marked.min.js", "purify.min.js", "highlight.min.js",
		"github-markdown.min.css", "github.min.css", "github-dark.min.css"} {
		response, body := get(t, handler, assetRoute+name)
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", name, response.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("%s: served no content", name)
		}
	}
}

func TestServeOnlineReferencesTheCDNs(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: cdnAssets, online: true}

	_, page := get(t, handler, "/")
	for _, want := range []string{cdnAssets.MarkedJS, cdnAssets.DOMPurifyJS, cdnAssets.HighlightJS,
		cdnAssets.MarkdownCSS, cdnAssets.HighlightCSSLite, cdnAssets.HighlightCSSDark} {
		if !strings.Contains(page, want) {
			t.Errorf("the online page does not reference %q", want)
		}
	}

	// The embedded copies are not served once the page loads them from the CDNs.
	response, _ := get(t, handler, assetRoute+"marked.min.js")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestServeDirSourceListsTheDirectory(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindDir}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	response, page := get(t, handler, "/docs")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if !strings.Contains(page, "<title>docs</title>") {
		t.Errorf("the page is not titled after the directory: %q", page)
	}

	_, body := get(t, handler, "/content?path=/docs")
	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Text == nil || !strings.Contains(*payload.Text, "- [guide.md](guide.md)") {
		t.Errorf("text = %v, want a listing", payload.Text)
	}
}

func TestServeFileNamespaceReachesAFileFromADirSource(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindDir}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	_, body := get(t, handler, "/content?path=/_/file/docs/guide.md")
	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Text == nil || *payload.Text != "# Guide\n" {
		t.Errorf("text = %v, want the guide", payload.Text)
	}
}

func TestServePageShellCarriesTheOutlineStyle(t *testing.T) {
	root := documentRoot(t)

	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}
	if _, page := get(t, handler, "/"); strings.Contains(page, `id="_outline"`) {
		t.Errorf("the page holds an outline without the flag: %q", page)
	}

	for _, style := range outlineStyles {
		handler.outline = outlineSettings{Style: style.name, Justify: "left"}
		_, page := get(t, handler, "/")
		if !strings.Contains(page, `<nav id="_outline" class="outline-`+style.name+`">`) {
			t.Errorf("the %s page does not hold its outline: %q", style.name, page)
		}
		if !strings.Contains(page, `<body class="with-outline outline-left">`) {
			t.Errorf("the %s page does not switch the layout: %q", style.name, page)
		}
	}

	handler.outline = outlineSettings{Style: "plain", Justify: "right"}
	if _, page := get(t, handler, "/"); !strings.Contains(page, `<body class="with-outline outline-right">`) {
		t.Errorf("the page does not put the outline on the right: %q", page)
	}
}

func TestServePageShellTakesTheOutlineFromTheQuery(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, outline: outlineSettings{Style: "numbered", Justify: "left"}}

	tests := map[string]string{
		"/?outline=style:plain":           `class="outline-plain"`,
		"/?outline=style:nh":              `class="outline-numbered-hierarchical"`,
		"/?outline=justify:right":         `class="with-outline outline-right"`,
		"/?outline=style:p,justify:right": `class="outline-plain"`,
		"/?outline=style:elsewhere":       `class="outline-numbered"`,
		"/?outline=colour:red":            `class="outline-numbered"`,
		"/":                               `class="outline-numbered"`,
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			if _, page := get(t, handler, target); !strings.Contains(page, want) {
				t.Errorf("the page does not hold %q: %q", want, page)
			}
		})
	}

	if _, page := get(t, handler, "/?outline=style:none"); strings.Contains(page, `id="_outline"`) {
		t.Errorf("the page holds an outline the query turned off: %q", page)
	}
}

func TestParseOutline(t *testing.T) {
	tests := map[string]outlineSettings{
		"style:plain":           {Style: "plain", Justify: "left"},
		"style:nh":              {Style: "numbered-hierarchical", Justify: "left"},
		"justify:right":         {Style: "numbered", Justify: "right"},
		"style:p,justify:right": {Style: "plain", Justify: "right"},
		"style:none":            {Style: "", Justify: "left"},
		"":                      {Style: "numbered", Justify: "left"},
		"justify:left,style:n,": {Style: "numbered", Justify: "left"},
	}
	given := outlineSettings{Style: "numbered", Justify: "left"}

	for list, want := range tests {
		t.Run(list, func(t *testing.T) {
			got, err := parseOutline(list, given)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("parseOutline(%q) = %+v, want %+v", list, got, want)
			}
		})
	}

	for _, list := range []string{"plain", "style:sideways", "justify:middle", "colour:red"} {
		t.Run(list, func(t *testing.T) {
			if _, err := parseOutline(list, given); err == nil {
				t.Errorf("parseOutline(%q) raised no error", list)
			}
		})
	}
}

func TestOutlineStyle(t *testing.T) {
	tests := map[string]struct {
		style string
		known bool
	}{
		"plain":                 {"plain", true},
		"p":                     {"plain", true},
		"numbered":              {"numbered", true},
		"n":                     {"numbered", true},
		"numbered-hierarchical": {"numbered-hierarchical", true},
		"nh":                    {"numbered-hierarchical", true},
		"none":                  {"", true},
		"":                      {"", false},
		"hierarchical":          {"", false},
		"P":                     {"", false},
	}

	for given, want := range tests {
		t.Run(given, func(t *testing.T) {
			style, known := outlineStyle(given)
			if style != want.style || known != want.known {
				t.Errorf("outlineStyle(%q) = %q, %v, want %q, %v", given, style, known, want.style, want.known)
			}
		})
	}
}

func TestServePageShellCarriesMermaidOnlyWhenAsked(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	if _, page := get(t, handler, "/"); strings.Contains(page, "mermaid.min.js") {
		t.Errorf("the page loads mermaid without the flag: %q", page)
	}

	handler.mermaid = true
	if _, page := get(t, handler, "/"); !strings.Contains(page, embeddedAssets.MermaidJS) {
		t.Errorf("the page does not name the mermaid bundle: %q", page)
	}
}

func TestServePageAssets(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: cdnAssets, online: true}

	// They are the server's own, so they are served from the binary even with --online.
	for _, name := range []string{"page.css", "page.js"} {
		response, body := get(t, handler, pageAssetRoute+name)
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", name, response.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("%s: served no content", name)
		}
	}
}

func TestServeRawAssets(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	tests := []struct {
		route       string
		contentType string
		want        string
	}{
		{"/picture.png", "image/png", pictureContent},
		{"/docs/drawing.svg", "image/svg+xml", drawingContent},
	}
	for _, test := range tests {
		t.Run(test.route, func(t *testing.T) {
			response, body := get(t, handler, test.route)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.StatusCode)
			}
			if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, test.contentType) {
				t.Errorf("Content-Type = %q, want %s", contentType, test.contentType)
			}
			if policy := response.Header.Get("Content-Security-Policy"); policy != "sandbox" {
				t.Errorf("Content-Security-Policy = %q, want sandbox", policy)
			}
			if body != test.want {
				t.Errorf("body = %q, want the file itself", body)
			}
		})
	}
}

func TestServeRawAssetRejectsTraversal(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: filepath.Join(root, "docs"),
		watchInterval: 1, assets: embeddedAssets}

	response, _ := get(t, handler, "/../picture.png")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestServeRejectsTraversal(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindFile}, rootDir: filepath.Join(root, "docs"),
		watchInterval: 1, assets: embeddedAssets}

	response, _ := get(t, handler, "/../notes.md")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}

	_, body := get(t, handler, "/content?path=/../notes.md")
	var payload contentPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Text != nil {
		t.Errorf("a file outside the root was served: %v", payload.Text)
	}
}
