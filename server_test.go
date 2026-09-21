package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

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
}

func TestServeEmbeddedAssetsByDefault(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
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

func TestServeDirectoryReadsItsDocumentOrListsIt(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	// A directory holding index.md or README.md is that document, written with a trailing
	// slash or without one.
	for _, route := range []string{"/docs", "/docs/"} {
		response, page := get(t, handler, route)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", route, response.StatusCode)
		}
		if !strings.Contains(page, "<title>index.md</title>") {
			t.Errorf("%s: the page is not titled after the document: %q", route, page)
		}
	}

	// A directory holding neither is listed, and one holding no document at all says so.
	tests := map[string]string{
		"/content?path=/listing": "- [one.md](one.md)",
		"/content?path=/empty":   "No Markdown files found.",
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			_, body := get(t, handler, target)
			var payload contentPayload
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Text == nil || !strings.Contains(*payload.Text, want) {
				t.Errorf("text = %v, want one holding %q", payload.Text, want)
			}
		})
	}
}

func TestServeContentCarriesTheDocumentBase(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	// A route naming a folder is answered with the document inside it, and its links are read
	// from that folder rather than from beside it, whatever the route's shape.
	tests := map[string]string{
		"/content?path=/docs/guide.md": "/docs/",
		"/content?path=/docs":          "/docs/",
		"/content?path=/docs/":         "/docs/",
		"/content?path=/":              "/",
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			_, body := get(t, handler, target)
			var payload contentPayload
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Base == nil || *payload.Base != want {
				t.Errorf("base = %v, want %q", payload.Base, want)
			}
		})
	}
}

func TestServeFileNamespaceReachesAFileFromADirSource(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1, assets: embeddedAssets}

	_, body := get(t, handler, "/content?path=/docs/guide.md")
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

	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}
	if _, page := get(t, handler, "/"); strings.Contains(page, `id="_outline"`) {
		t.Errorf("the page holds an outline without the flag: %q", page)
	}

	for _, style := range outlineStyles {
		handler.outline = outlineSettings{Style: style.name, Justify: "left"}
		_, page := get(t, handler, "/")
		if !strings.Contains(page, `<nav id="_outline" class="sidebar style-`+style.name+`">`) {
			t.Errorf("the %s page does not hold its outline: %q", style.name, page)
		}
		if !strings.Contains(page, `<body class="with-sidebar outline-left">`) {
			t.Errorf("the %s page does not switch the layout: %q", style.name, page)
		}
	}

	handler.outline = outlineSettings{Style: "plain", Justify: "right"}
	if _, page := get(t, handler, "/"); !strings.Contains(page, `<body class="with-sidebar outline-right">`) {
		t.Errorf("the page does not put the outline on the right: %q", page)
	}
}

func TestServePageShellTakesTheOutlineFromTheQuery(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, outline: outlineSettings{Style: "numbered", Justify: "left"}}

	tests := map[string]string{
		"/?outline=style:plain":           `class="sidebar style-plain"`,
		"/?outline=style:nh":              `class="sidebar style-numbered-hierarchical"`,
		"/?outline=justify:right":         `class="with-sidebar outline-right"`,
		"/?outline=style:p,justify:right": `class="sidebar style-plain"`,
		"/?outline=style:elsewhere":       `class="sidebar style-numbered"`,
		"/?outline=colour:red":            `class="sidebar style-numbered"`,
		"/":                               `class="sidebar style-numbered"`,
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

func TestServePageShellCarriesTheDirectoryList(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, outline: outlineSettings{Style: "numbered", Justify: "left"},
		list: listSettings{Style: "plain", Scope: "current"}}

	_, page := get(t, handler, "/docs/guide.md")
	for _, want := range []string{
		`<nav id="_list" class="sidebar style-plain">`,
		`<a href="/docs/guide.md" class="current">guide.md</a>`,
		`<a href="/docs/index.md">index.md</a>`,
		// The list takes the outline to the right, whatever the outline was given.
		`<body class="with-sidebar outline-right">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}

	// An ado source the server cannot read holds no list, so its outline keeps its own side.
	ado := &server{defaultSource: source{kind: kindADO, organization: "org", project: "proj",
		repository: "repo", path: "/README.md"}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, outline: outlineSettings{Style: "numbered", Justify: "left"},
		list: listSettings{Style: "plain", Scope: "current"}}
	if _, page := get(t, ado, "/notes.md"); !strings.Contains(page, `id="_list"`) {
		t.Errorf("the local namespace holds no list under an ado source: %q", page)
	}

	// The query turns the list off and the outline goes back to its own side.
	if _, page := get(t, handler, "/docs/guide.md?list=style:none"); strings.Contains(page, `id="_list"`) ||
		!strings.Contains(page, `<body class="with-sidebar outline-left">`) {
		t.Errorf("the query did not turn the list off: %q", page)
	}

	// The scope reaches further when the query asks it to, and browsing keeps that query.
	if _, page := get(t, handler, "/docs/guide.md?list=scope:tree"); !strings.Contains(page,
		`<a href="/plain/README.md?list=scope:tree">README.md</a>`) {
		t.Errorf("the tree scope does not reach the whole source: %q", page)
	}
}

func TestServePageShellTakesASidebarFromTheQueryAlone(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, outline: outlineSettings{Justify: "left"},
		list: listSettings{Scope: "current"}}

	// The server was started without either sidebar; a query naming any setting draws the one
	// it belongs to, with the default style, the way the flags do.
	tests := map[string]string{
		"/docs/guide.md?outline=justify:right": `<nav id="_outline" class="sidebar style-plain">`,
		"/docs/guide.md?list=scope:current":    `<nav id="_list" class="sidebar style-plain">`,
		"/docs/guide.md?list=scope:tree":       `<a href="/plain/README.md?list=scope:tree">README.md</a>`,
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			if _, page := get(t, handler, target); !strings.Contains(page, want) {
				t.Errorf("the page does not hold %q: %q", want, page)
			}
		})
	}

	// A query that does not read draws neither.
	if _, page := get(t, handler, "/docs/guide.md?list=colour:red&outline=sideways"); strings.Contains(page, "sidebar") {
		t.Errorf("an unread query drew a sidebar: %q", page)
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
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
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: filepath.Join(root, "docs"),
		watchInterval: 1, assets: embeddedAssets}

	response, _ := get(t, handler, "/../picture.png")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestServeRejectsTraversal(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: filepath.Join(root, "docs"),
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

func TestServeRefusesTraversalFromEveryRoute(t *testing.T) {
	root := documentRoot(t)
	// A document the server must never reach, beside the directory it was started in.
	writeFile(t, filepath.Join(filepath.Dir(root), "secret.md"), "# Secret\n")
	writeFile(t, filepath.Join(filepath.Dir(root), "secret.png"), pictureContent)

	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, list: listSettings{Style: "plain", Scope: "tree"}}

	// A '..' that stays inside the root is followed, the way a link in a document writes it.
	inside := map[string]string{
		"/docs/../notes.md":        "# Notes\n",
		"/docs/../docs/guide.md":   "# Guide\n",
		"/docs/subdir/../guide.md": "# Guide\n",
		"/./docs/./guide.md":       "# Guide\n",
		"/docs/%2e%2e/notes.md":    "# Notes\n",
		"/plain/../docs/../docs/":  "# Docs\n",
	}
	for route, want := range inside {
		t.Run("inside "+route, func(t *testing.T) {
			_, body := get(t, handler, "/content?path="+url.QueryEscape(route))
			var payload contentPayload
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Text == nil || *payload.Text != want {
				t.Errorf("text = %v, want %q", payload.Text, want)
			}
		})
	}

	// A '..' that climbs out of the root reaches nothing, however it is written.
	outside := []string{
		"/../secret.md",
		"/docs/../../secret.md",
		"/docs/../..",
		"/%2e%2e/secret.md",
		"/docs/%2e%2e/%2e%2e/secret.md",
		"/..%2fsecret.md",
	}
	for _, route := range outside {
		t.Run("outside "+route, func(t *testing.T) {
			response, page := get(t, handler, route)
			if response.StatusCode != http.StatusNotFound {
				t.Errorf("the page route answered %d, want 404: %q", response.StatusCode, page)
			}
			if strings.Contains(page, "Secret") {
				t.Errorf("the page route served the document outside the root: %q", page)
			}

			_, body := get(t, handler, "/content?path="+url.QueryEscape(route))
			var payload contentPayload
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Text != nil {
				t.Errorf("/content served a document outside the root: %q", *payload.Text)
			}
			if payload.Error == nil {
				t.Error("/content reported no error for a route outside the root")
			}
		})
	}

	// The same holds for the pictures served as their own bytes.
	pictures := map[string]int{
		"/docs/../picture.png":   http.StatusOK,
		"/../secret.png":         http.StatusNotFound,
		"/docs/../../secret.png": http.StatusNotFound,
		"/%2e%2e/secret.png":     http.StatusNotFound,
	}
	for route, want := range pictures {
		t.Run("picture "+route, func(t *testing.T) {
			response, body := get(t, handler, route)
			if response.StatusCode != want {
				t.Errorf("status = %d, want %d", response.StatusCode, want)
			}
			if want != http.StatusOK && strings.Contains(body, pictureContent) {
				t.Error("a picture outside the root was served")
			}
		})
	}

	// And for the directory list, which must name nothing above the root.
	entries, _ := handler.documentList(source{kind: kindLocal, route: "/docs/../.."}, "tree")
	for _, entry := range entries {
		t.Errorf("the list names %q from outside the root", entry.Route)
	}
}

func TestServeADOOrganizationAndProjectPages(t *testing.T) {
	root := documentRoot(t)
	// Without --list nothing is read from Azure DevOps to build the page shell, so the titles
	// are answered whatever the network does.
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	tests := map[string]string{
		"/_/ado/myorg":                "<title>myorg</title>",
		"/_/ado/myorg/myproject":      "<title>myproject</title>",
		"/_/ado/myorg/myproject/repo": "<title>repo</title>",
	}
	for route, want := range tests {
		t.Run(route, func(t *testing.T) {
			response, page := get(t, handler, route)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.StatusCode)
			}
			if !strings.Contains(page, want) {
				t.Errorf("the page does not hold %q: %q", want, page)
			}
		})
	}

	// A route naming no organization reaches nothing.
	if response, _ := get(t, handler, "/_/ado/"); response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestServePageShellChecksLocalDocumentsAlone(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets}

	// A local page re-reads its document on its own; one reading Azure Repos waits for the
	// browser's refresh, the document being read over the network.
	if _, page := get(t, handler, "/"); !strings.Contains(page, `"watchIntervalMS":1000`) {
		t.Errorf("the local page does not check for changes: %q", page)
	}
	if _, page := get(t, handler, "/_/ado/myorg/myproject/repo"); !strings.Contains(page, `"watchIntervalMS":0`) {
		t.Errorf("the ado page checks for changes: %q", page)
	}
}

func TestServePageShellCarriesTheADOVersion(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, ado: adoSettings{Version: "main", VersionType: "branch"}}

	// The page polls at the version it was asked for, so the document and the shell agree.
	_, page := get(t, handler, "/_/ado/myorg/myproject/repo?ado=tag:v1.0")
	// The parameters are read apart, the separator between them being escaped as JSON in HTML.
	for _, want := range []string{
		`"contentQuery":"?path=%2F_%2Fado%2Fmyorg%2Fmyproject%2Frepo`,
		`ado=tag%3Av1.0"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}

	// Without a parameter the page polls for the route alone, reading at the server's version.
	_, page = get(t, handler, "/_/ado/myorg/myproject/repo")
	if strings.Contains(page, "ado=") {
		t.Errorf("the page carries a version it was not asked for: %q", page)
	}

	// The version reaches the source the page's sidebars and documents are read from.
	src, _ := resolveSource("/_/ado/myorg/myproject/repo", handler.defaultSource)
	if read := handler.adoVersion(src, "tag:v1.0"); read.version != "v1.0" || read.versionType != "tag" {
		t.Errorf("source = %+v, want the tag the page named", read)
	}
	if read := handler.adoVersion(src, "sideways"); read.version != "main" || read.versionType != "branch" {
		t.Errorf("an unread parameter changed the version to %+v", read)
	}
	local, _ := resolveSource("/notes.md", handler.defaultSource)
	if read := handler.adoVersion(local, "tag:v1.0"); read.version != "" {
		t.Errorf("a local source carries the version %q", read.version)
	}
}

func TestServePageShellCarriesTheListLink(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		assets: embeddedAssets, list: listSettings{Style: "plain", Scope: "current"}}

	// The link above a repository's documents leads back to the repositories of its project,
	// carrying the settings the page was opened with.
	beside := sidebars{List: listSettings{Style: "plain"},
		Up: adoUpLink(source{kind: kindADO, organization: "my org", project: "my proj",
			repository: "repo", path: "/docs/guide.md"})}
	if !beside.HoldsList() {
		t.Error("a list holding only the link above it is left out of the page")
	}

	page := string(renderPage("guide.md", "?path=/", 1000, embeddedAssets,
		pageSettings{Sidebars: beside, ContentWidth: defaultContentWidth}))
	if want := `<a class="up" href="/_/ado/my%20org/my%20proj">Browse to repositories</a>`; !strings.Contains(page, want) {
		t.Errorf("the page does not hold %q: %q", want, page)
	}

	// A list of repositories leads to the same page, under the label naming what it holds.
	up := adoUpLink(source{kind: kindADO, organization: "my org", project: "my proj",
		repository: "repo"})
	up.Label = "Back to projects"
	page = string(renderPage("README.md", "?path=/", 1000, embeddedAssets, pageSettings{
		Sidebars:     sidebars{List: listSettings{Style: "plain"}, Up: up},
		ContentWidth: defaultContentWidth}))
	if want := `<a class="up" href="/_/ado/my%20org/my%20proj">Back to projects</a>`; !strings.Contains(page, want) {
		t.Errorf("the page does not hold %q: %q", want, page)
	}

	// A local list has nothing above it.
	entries, up := handler.documentList(source{kind: kindLocal, route: "/docs/guide.md"}, "current")
	if len(entries) == 0 || up.Label != "" {
		t.Errorf("a local list carries the link %q above it", up.Label)
	}
}
