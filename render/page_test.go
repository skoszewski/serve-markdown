package render

import (
	"strings"
	"testing"
)

func TestBodyClass(t *testing.T) {
	outline := Sidebars{Outline: OutlineSettings{Style: "plain", Justify: "right"}}

	tests := map[string]struct {
		settings PageSettings
		want     string
	}{
		// The window needs no rule capping it, so it carries no class of its own.
		"the width it is given unasked": {PageSettings{ContentWidth: WidthFull}, ""},
		"a width nobody set":            {PageSettings{}, ""},
		"a capped width":                {PageSettings{ContentWidth: "medium"}, "width-medium"},
		"a capped width beside an outline": {PageSettings{ContentWidth: "small", Sidebars: outline},
			"width-small with-sidebar outline-right"},
		"the window beside an outline": {PageSettings{ContentWidth: WidthFull, Sidebars: outline},
			"with-sidebar outline-right"},
		"no separators":           {PageSettings{Separators: "hidden"}, "separators-hidden"},
		"the top separator alone": {PageSettings{Separators: "top"}, "separators-top"},
		"the side separators":     {PageSettings{Separators: "side"}, "separators-side"},
		"every separator":         {PageSettings{Separators: "all"}, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.settings.bodyClass(); got != test.want {
				t.Errorf("bodyClass = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPageNamesItsAssetsUnderThePrefix(t *testing.T) {
	page := string(Page("guide.md", "/docs/_/content?path=%2Fdocs%2F", 0,
		PageSettings{AssetPrefix: "/docs/_/", Banner: "Docs", Mermaid: true}))
	for _, want := range []string{
		`href="/docs/_/assets/github-markdown-light.css"`,
		`href="/docs/_/page/page.css"`,
		`src="/docs/_/assets/page.js"`,
		`"contentURL":"/docs/_/content?path=%2Fdocs%2F"`,
		`"mermaidJS":"/docs/_/assets/mermaid.min.js"`,
		`<span id="_banner">Docs</span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}
}
