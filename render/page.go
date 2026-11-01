// Package render draws Markdown documents as the pages of serve-markdown: a page shell that
// fetches its document as JSON and renders it in the browser, with the document's front matter,
// an outline of its headings, a directory list or a menu beside it, a light and a dark theme,
// and mermaid diagrams.
//
// Site serves a set of documents in an fs.FS under a route prefix of its own; the other
// functions are the parts it is made of, for a server reading its documents another way.
package render

import (
	"bytes"
	"html/template"
	"strings"
)

// WidthFull is the width that caps the document at nothing, giving it the window.
const WidthFull = "full"

// SeparatorsAll draws every line, which the page does without a class of its own.
const SeparatorsAll = "all"

// Hidden is the display that hides the outline or the list until the reader shows it.
const Hidden = "hidden"

// DefaultOutline is what an outline is drawn with unless told otherwise, and DefaultList the
// same for a directory list.
var (
	DefaultOutline = OutlineSettings{Style: "plain", Justify: "left", Display: "shown"}
	DefaultList    = ListSettings{Scope: "current", Display: "shown"}
)

// assetURLs names the built assets the page shell loads. The fields are read by the page
// template.
type assetURLs struct {
	MarkdownCSSLite  string
	MarkdownCSSDark  string
	HighlightCSSLite string
	HighlightCSSDark string
	PageJS           string
	MermaidJS        string
}

// pageConfig is what the page script reads its settings from. ContentURL is the URL the
// document is fetched from. MermaidJS is empty unless diagrams are drawn, and a fenced mermaid
// block then stays a code block. OutlineHidden and ListHidden hide the outline and the
// directory list until the reader chooses otherwise. Print draws the page in the light theme,
// with the links into the server left as text, to be printed to a PDF.
type pageConfig struct {
	ContentURL      string         `json:"contentURL"`
	WatchIntervalMS int            `json:"watchIntervalMS"`
	MermaidJS       string         `json:"mermaidJS"`
	OutlineHidden   bool           `json:"outlineHidden"`
	ListHidden      bool           `json:"listHidden"`
	Print           bool           `json:"print"`
	Versions        *VersionPicker `json:"versions"`
}

// VersionPicker is what the page draws above a document read from a repository: the branch
// the repository is read at unasked, the branches and tags it holds, and which of them the page
// was asked for.
//
// Both lists travel with the page, so choosing between branches and tags asks the server for
// nothing. Kind is "branch" or "tag", empty where no version was asked for.
type VersionPicker struct {
	DefaultBranch string   `json:"defaultBranch"`
	Branches      []string `json:"branches"`
	Tags          []string `json:"tags"`
	Kind          string   `json:"kind"`
	Version       string   `json:"version"`
}

// OutlineSettings is how a page's outline is drawn: the style its entries are numbered in,
// empty for no outline at all, the side of the document it stands on, and whether it is shown
// or hidden before the reader chooses.
type OutlineSettings struct {
	Style   string
	Justify string
	Display string
}

// ListSettings is how a page's directory list is drawn: how much of the source it reaches,
// empty for no list at all, and whether it is shown or hidden before the reader chooses.
type ListSettings struct {
	Scope   string
	Display string
}

// ListEntry is one line of the directory list: a document or a folder to open, holding the
// entries of the folder below it.
type ListEntry struct {
	Name     string
	Route    string
	Current  bool
	Children []ListEntry
}

// ListLink is the link standing above the directory list, leading out of what the list holds.
type ListLink struct {
	Label string
	Route string
}

// Sidebars is what stands beside the document: the outline built from its headings, and the
// list of the documents around it, under a link out of them. Menu tells that the entries are a
// menu rather than a directory list.
type Sidebars struct {
	Outline OutlineSettings
	List    ListSettings
	Up      ListLink
	Entries []ListEntry
	Menu    bool
}

// HoldsList reports whether the page carries a directory list: its entries, the link above
// them, or both. It is read by the page template.
func (s Sidebars) HoldsList() bool {
	return len(s.Entries) > 0 || s.Up.Label != ""
}

// PageSettings is what a page is drawn with, beyond the document it shows.
//
// AssetPrefix is the route the assets are served under, ending in a slash, as AssetHandler is
// given it, and Banner the text beside the control buttons, none when empty. Sidebars stand
// beside the document, ContentWidth says how wide it is drawn, Separators which lines between
// its boxes are, and Mermaid whether its diagrams are. Print draws it to be printed to a PDF,
// its text in PrintFontSize, empty for the stylesheet's own.
type PageSettings struct {
	AssetPrefix   string
	Banner        string
	Sidebars      Sidebars
	ContentWidth  string
	Separators    string
	Mermaid       bool
	Versions      *VersionPicker
	Print         bool
	PrintFontSize string
}

// bodyClass returns the classes the page's body carries: how wide the document is drawn, the
// sidebars it holds, the side the outline stands on, and the separators it draws.
//
// The width carries no class where the document is given the window, which is the width it
// has without a rule capping it, and the separators none where every one is drawn.
func (p PageSettings) bodyClass() string {
	var classes []string
	if p.ContentWidth != "" && p.ContentWidth != WidthFull {
		classes = append(classes, "width-"+p.ContentWidth)
	}
	if p.Sidebars.Outline.Style != "" || p.Sidebars.HoldsList() {
		classes = append(classes, "with-sidebar")
	}
	if p.Sidebars.Outline.Style != "" {
		classes = append(classes, "outline-"+p.Sidebars.Outline.Justify)
	}
	if p.Separators != "" && p.Separators != SeparatorsAll {
		classes = append(classes, "separators-"+p.Separators)
	}
	return strings.Join(classes, " ")
}

// pageData is what the page template renders.
type pageData struct {
	Title         string
	Banner        string
	Assets        assetURLs
	Sidebars      Sidebars
	BodyClass     string
	PageCSS       string
	PageCSSDark   string
	PrintFontSize string
	Config        pageConfig
}

var pageTemplate = template.Must(template.ParseFS(pageFS, "page/page.html"))

// Page builds the HTML page shell that fetches its document from contentURL and renders it as
// Markdown.
//
// watchIntervalMS is the milliseconds between fetches, zero for a page that fetches once. A
// sidebar with an empty style, or a list without entries, is left out of the page.
func Page(title, contentURL string, watchIntervalMS int, settings PageSettings) []byte {
	built := settings.AssetPrefix + "assets/"
	data := pageData{
		Title:  title,
		Banner: settings.Banner,
		Assets: assetURLs{
			MarkdownCSSLite:  built + "github-markdown-light.css",
			MarkdownCSSDark:  built + "github-markdown-dark.css",
			HighlightCSSLite: built + "highlight-light.css",
			HighlightCSSDark: built + "highlight-dark.css",
			PageJS:           built + "page.js",
			MermaidJS:        built + "mermaid.min.js",
		},
		Sidebars:      settings.Sidebars,
		BodyClass:     settings.bodyClass(),
		PageCSS:       settings.AssetPrefix + "page/page.css",
		PageCSSDark:   settings.AssetPrefix + "page/page-dark.css",
		PrintFontSize: settings.PrintFontSize,
		Config: pageConfig{ContentURL: contentURL, WatchIntervalMS: watchIntervalMS,
			OutlineHidden: settings.Sidebars.Outline.Display == Hidden,
			ListHidden:    settings.Sidebars.List.Display == Hidden, Print: settings.Print,
			Versions: settings.Versions},
	}
	if settings.Mermaid {
		data.Config.MermaidJS = data.Assets.MermaidJS
	}

	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, data); err != nil {
		panic(err)
	}
	return page.Bytes()
}
