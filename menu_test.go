package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadMenuLeavesOutWhatIsNotServed(t *testing.T) {
	root := documentRoot(t)
	writeFile(t, filepath.Join(root, "drafts", "draft.md"), "# Draft\n")
	rules := ignoring(t, "drafts/")
	rules.rootDir = root
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, ignore: rules}

	menu, warnings := handler.readMenu([]menuEntry{
		{Label: "Home", Path: "README.md", line: 2},
		{Label: "Missing", Path: "nowhere.md", line: 3},
		{Label: "Folder", Path: "docs", line: 4},
		{Label: "Guides", line: 5, Entries: []menuEntry{
			{Label: "Guide", Path: "docs/guide.md", line: 6},
			{Label: "Draft", Path: "drafts/draft.md", line: 7},
		}},
		{Label: "Empty", line: 8, Entries: []menuEntry{{Label: "Outside", Path: "../secret.md", line: 9}}},
	})
	if len(menu) != 2 || menu[0].Route != "/README.md" || menu[1].Name != "Guides" || menu[1].Route != "" ||
		len(menu[1].Children) != 1 || menu[1].Children[0].Route != "/docs/guide.md" {
		t.Errorf("menu = %+v, want Home and Guides holding Guide", menu)
	}
	for _, want := range []string{"line 3: 'Missing'", "line 4: 'Folder'", "line 7: 'Draft'", "line 9: 'Outside'",
		"line 8: the group 'Empty'"} {
		if !strings.Contains(strings.Join(warnings, "\n"), want) {
			t.Errorf("warnings = %q, want one holding %q", warnings, want)
		}
	}
}

func TestServePageShellCarriesTheMenu(t *testing.T) {
	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		list: listSettings{Scope: "current", Display: "shown"}}
	handler.menu, _ = handler.readMenu([]menuEntry{
		{Label: "Home", Path: "README.md"},
		{Label: "Guides", Entries: []menuEntry{{Label: "The guide", Path: "docs/guide.md"}}},
	})

	_, page := get(t, handler, "/docs/guide.md?list=scope:tree")
	for _, want := range []string{
		`<a href="/README.md?list=scope:tree">Home</a>`,
		`<span>Guides</span><ol><li><a href="/docs/guide.md?list=scope:tree" class="current">The guide</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not hold %q: %q", want, page)
		}
	}
	// The menu stands in place of the list, whatever the query asks of it.
	if strings.Contains(page, "index.md") || strings.Contains(page, "plain") {
		t.Errorf("the page lists the folders beside the menu: %q", page)
	}

	// The served directory's own route is read as its README.md, whose entry is marked.
	if _, page := get(t, handler, "/"); !strings.Contains(page, `<a href="/README.md" class="current">Home</a>`) {
		t.Errorf("the entry of the directory's document is not marked: %q", page)
	}
}
