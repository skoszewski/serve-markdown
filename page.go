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

// pageData is what the page template renders.
type pageData struct {
	Title   string
	Assets  assetURLs
	Outline outlineSettings
	PageCSS string
	PageJS  string
	Config  pageConfig
}

var pageTemplate = template.Must(template.ParseFS(pageFS, "assets/page/page.html"))

// renderPage builds the HTML page shell that polls /content and renders it as Markdown.
//
// contentQuery is the query string, including its leading '?', appended to the /content
// request, and watchIntervalMS the milliseconds between polls. The outline's style is empty
// for a page without one.
func renderPage(title, contentQuery string, watchIntervalMS int, assets assetURLs, outline outlineSettings, mermaid bool) []byte {
	data := pageData{
		Title:   title,
		Assets:  assets,
		Outline: outline,
		PageCSS: pageAssetRoute + "page.css",
		PageJS:  pageAssetRoute + "page.js",
		Config:  pageConfig{ContentQuery: contentQuery, WatchIntervalMS: watchIntervalMS},
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
