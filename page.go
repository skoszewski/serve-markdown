package main

import (
	"bytes"
	"html/template"
)

// assetURLs names the browser-side libraries the page shell loads. The fields are read by
// the page template.
type assetURLs struct {
	MarkdownCSS      string
	HighlightCSSLite string
	HighlightCSSDark string
	MarkedJS         string
	DOMPurifyJS      string
	HighlightJS      string
	MermaidJS        string
}

// cdnAssets loads the libraries from their public CDNs, as --online asks for.
var cdnAssets = assetURLs{
	MarkdownCSS:      "https://cdn.jsdelivr.net/npm/github-markdown-css@5/github-markdown.min.css",
	HighlightCSSLite: "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css",
	HighlightCSSDark: "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css",
	MarkedJS:         "https://cdn.jsdelivr.net/npm/marked@15/marked.min.js",
	DOMPurifyJS:      "https://cdn.jsdelivr.net/npm/dompurify@3/dist/purify.min.js",
	HighlightJS:      "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js",
	MermaidJS:        "https://cdn.jsdelivr.net/npm/mermaid@12/dist/mermaid.min.js",
}

// embeddedAssets loads the same libraries from the copies built into the binary.
var embeddedAssets = assetURLs{
	MarkdownCSS:      assetRoute + "github-markdown.min.css",
	HighlightCSSLite: assetRoute + "github.min.css",
	HighlightCSSDark: assetRoute + "github-dark.min.css",
	MarkedJS:         assetRoute + "marked.min.js",
	DOMPurifyJS:      assetRoute + "purify.min.js",
	HighlightJS:      assetRoute + "highlight.min.js",
	MermaidJS:        assetRoute + "mermaid.min.js",
}

// pageConfig is what the page script reads its settings from. MermaidJS is empty unless
// --mermaid is given, and a fenced mermaid block then stays a code block.
type pageConfig struct {
	ContentQuery    string `json:"contentQuery"`
	WatchIntervalMS int    `json:"watchIntervalMS"`
	MermaidJS       string `json:"mermaidJS"`
}

// outlineSettings is how a page's outline is drawn: the style its entries are numbered in,
// empty for no outline at all, and the side of the document it stands on.
type outlineSettings struct {
	Style   string
	Justify string
}

// listSettings is how a page's directory list is drawn: the style its entries are numbered
// in, empty for no list at all, and how much of the source it reaches.
type listSettings struct {
	Style string
	Scope string
}

// listEntry is one line of the directory list: a document or a folder to open, holding the
// entries of the folder below it.
type listEntry struct {
	Name     string
	Route    string
	Current  bool
	Children []listEntry
}

// listLink is the link standing above the directory list, leading out of what the list holds.
type listLink struct {
	Label string
	Route string
}

// sidebars is what stands beside the document: the outline built from its headings, and the
// list of the documents around it, under a link out of them.
type sidebars struct {
	Outline outlineSettings
	List    listSettings
	Up      listLink
	Entries []listEntry
}

// HoldsList reports whether the page carries a directory list: its entries, the link above
// them, or both. It is read by the page template.
func (s sidebars) HoldsList() bool {
	return len(s.Entries) > 0 || s.Up.Label != ""
}

// bodyClass returns the class the page's body carries, naming the sidebars it holds and the
// side the outline stands on.
func (s sidebars) bodyClass() string {
	if s.Outline.Style == "" && !s.HoldsList() {
		return ""
	}
	if s.Outline.Style == "" {
		return "with-sidebar"
	}
	return "with-sidebar outline-" + s.Outline.Justify
}

// pageData is what the page template renders.
type pageData struct {
	Title     string
	Assets    assetURLs
	Sidebars  sidebars
	BodyClass string
	PageCSS   string
	PageJS    string
	Config    pageConfig
}

var pageTemplate = template.Must(template.ParseFS(pageFS, "assets/page/page.html"))

// renderPage builds the HTML page shell that polls /content and renders it as Markdown.
//
// contentQuery is the query string, including its leading '?', appended to the /content
// request, and watchIntervalMS the milliseconds between polls. A sidebar with an empty style,
// or a list without entries, is left out of the page.
func renderPage(title, contentQuery string, watchIntervalMS int, assets assetURLs, beside sidebars, mermaid bool) []byte {
	data := pageData{
		Title:     title,
		Assets:    assets,
		Sidebars:  beside,
		BodyClass: beside.bodyClass(),
		PageCSS:   pageAssetRoute + "page.css",
		PageJS:    pageAssetRoute + "page.js",
		Config:    pageConfig{ContentQuery: contentQuery, WatchIntervalMS: watchIntervalMS},
	}
	if mermaid {
		data.Config.MermaidJS = assets.MermaidJS
	}

	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, data); err != nil {
		panic(err)
	}
	return page.Bytes()
}
