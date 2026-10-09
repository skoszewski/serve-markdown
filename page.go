package main

import (
	"bytes"
	"html/template"
	"strings"
)

// assetURLs names the built assets the page shell loads. The fields are read by the page
// template.
type assetURLs struct {
	MarkdownCSS      string
	HighlightCSSLite string
	HighlightCSSDark string
	PageJS           string
	MermaidJS        string
}

// pageAssets loads the assets built into the binary: the stylesheets, the page script bundled
// with the libraries it uses, and mermaid.
var pageAssets = assetURLs{
	MarkdownCSS:      assetRoute + "github-markdown.css",
	HighlightCSSLite: assetRoute + "highlight-light.css",
	HighlightCSSDark: assetRoute + "highlight-dark.css",
	PageJS:           assetRoute + "page.js",
	MermaidJS:        assetRoute + "mermaid.min.js",
}

// pageConfig is what the page script reads its settings from. MermaidJS is empty unless
// --mermaid is given, and a fenced mermaid block then stays a code block. OutlineHidden and
// ListHidden hide the outline and the directory list until the reader chooses otherwise.
type pageConfig struct {
	ContentQuery    string         `json:"contentQuery"`
	WatchIntervalMS int            `json:"watchIntervalMS"`
	MermaidJS       string         `json:"mermaidJS"`
	OutlineHidden   bool           `json:"outlineHidden"`
	ListHidden      bool           `json:"listHidden"`
	Versions        *versionPicker `json:"versions"`
}

// versionPicker is what the page draws above a document read from Azure Repos: the branch the
// repository is read at unasked, the branches and tags it holds, and which of them the page
// was asked for.
//
// Both lists travel with the page, so choosing between branches and tags asks the server for
// nothing. Kind is "branch" or "tag", empty where no version was asked for.
type versionPicker struct {
	DefaultBranch string   `json:"defaultBranch"`
	Branches      []string `json:"branches"`
	Tags          []string `json:"tags"`
	Kind          string   `json:"kind"`
	Version       string   `json:"version"`
}

// outlineSettings is how a page's outline is drawn: the style its entries are numbered in,
// empty for no outline at all, the side of the document it stands on, and whether it is shown
// or hidden before the reader chooses.
type outlineSettings struct {
	Style   string
	Justify string
	Display string
}

// listSettings is how a page's directory list is drawn: how much of the source it reaches,
// empty for no list at all, and whether it is shown or hidden before the reader chooses.
type listSettings struct {
	Scope   string
	Display string
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

// pageSettings is what a page is drawn with, beyond the document it shows: what stands beside
// it, how wide it is drawn, whether the lines between its boxes are, and whether its diagrams
// are.
type pageSettings struct {
	Sidebars     sidebars
	ContentWidth string
	Separators   string
	Mermaid      bool
	Versions     *versionPicker
}

// bodyClass returns the classes the page's body carries: how wide the document is drawn, the
// sidebars it holds, the side the outline stands on, and the separators it draws.
//
// The width carries no class where the document is given the window, which is the width it
// has without a rule capping it, and the separators none where every one is drawn.
func (p pageSettings) bodyClass() string {
	var classes []string
	if p.ContentWidth != "" && p.ContentWidth != widthFull {
		classes = append(classes, "width-"+p.ContentWidth)
	}
	if p.Sidebars.Outline.Style != "" || p.Sidebars.HoldsList() {
		classes = append(classes, "with-sidebar")
	}
	if p.Sidebars.Outline.Style != "" {
		classes = append(classes, "outline-"+p.Sidebars.Outline.Justify)
	}
	if p.Separators != "" && p.Separators != separatorsAll {
		classes = append(classes, "separators-"+p.Separators)
	}
	return strings.Join(classes, " ")
}

// pageData is what the page template renders.
type pageData struct {
	Title     string
	Assets    assetURLs
	Sidebars  sidebars
	BodyClass string
	PageCSS   string
	Config    pageConfig
}

var pageTemplate = template.Must(template.ParseFS(pageFS, "assets/page/page.html"))

// renderPage builds the HTML page shell that polls /content and renders it as Markdown.
//
// contentQuery is the query string, including its leading '?', appended to the /content
// request, and watchIntervalMS the milliseconds between polls, zero for a page that makes
// none. A sidebar with an empty style, or a list without entries, is left out of the page.
func renderPage(title, contentQuery string, watchIntervalMS int, settings pageSettings) []byte {
	data := pageData{
		Title:     title,
		Assets:    pageAssets,
		Sidebars:  settings.Sidebars,
		BodyClass: settings.bodyClass(),
		PageCSS:   pageAssetRoute + "page.css",
		Config: pageConfig{ContentQuery: contentQuery, WatchIntervalMS: watchIntervalMS,
			OutlineHidden: settings.Sidebars.Outline.Display == outlineHidden,
			ListHidden:    settings.Sidebars.List.Display == outlineHidden, Versions: settings.Versions},
	}
	if settings.Mermaid {
		data.Config.MermaidJS = pageAssets.MermaidJS
	}

	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, data); err != nil {
		panic(err)
	}
	return page.Bytes()
}
