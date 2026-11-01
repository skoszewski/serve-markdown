package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skoszewski/serve-markdown/v3/render"
)

func TestMenuLeavesOutWhatIsNotServed(t *testing.T) {
	root := documentRoot(t)
	writeFile(t, filepath.Join(root, "drafts", "draft.md"), "# Draft\n")
	rules := ignoring(t, "drafts/")
	rules.rootDir = root
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, ignore: rules}

	menu, warnings := (&menuSource{}).resolve(handler, []render.MenuEntry{
		{Label: "Home", Path: "README.md", Line: 2},
		{Label: "Missing", Path: "nowhere.md", Line: 3},
		{Label: "Folder", Path: "docs", Line: 4},
		{Label: "Guides", Line: 5, Entries: []render.MenuEntry{
			{Label: "Guide", Path: "docs/guide.md", Line: 6},
			{Label: "Draft", Path: "drafts/draft.md", Line: 7},
		}},
		{Label: "Empty", Line: 8, Entries: []render.MenuEntry{{Label: "Outside", Path: "../secret.md", Line: 9}}},
	})
	if len(menu) != 2 || menu[0].Route != "/README.md" || menu[1].Name != "Guides" || menu[1].Route != "" ||
		len(menu[1].Children) != 1 || menu[1].Children[0].Route != "/docs/guide.md" {
		t.Errorf("menu = %+v, want Home and Guides holding Guide", menu)
	}
	for _, want := range []string{"line 3: 'nowhere.md'", "line 4: 'docs'", "line 7: 'drafts/draft.md'",
		"line 9: '../secret.md'", "line 8: the group 'Empty'"} {
		if !strings.Contains(strings.Join(warnings, "\n"), want) {
			t.Errorf("warnings = %q, want one holding %q", warnings, want)
		}
	}
}

func TestMenuLabelsADocumentByItsTitle(t *testing.T) {
	root := documentRoot(t)
	guide := filepath.Join(root, "docs", "guide.md")
	writeFile(t, guide, "---\ntitle: The Guide\n---\n\n# Guide\n")
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root}
	source := &menuSource{}

	writeFile(t, filepath.Join(root, "fenced.md"), "```md\n# Not a heading\n```\n\nText.\n\n# Fenced Out\n")
	writeFile(t, filepath.Join(root, "plain.md"), "Text alone.\n")
	menu, _ := source.resolve(handler, []render.MenuEntry{{Path: "docs/guide.md"}, {Path: "notes.md"},
		{Path: "fenced.md"}, {Path: "plain.md"}, {Label: "Written", Path: "docs/index.md"}})
	var names []string
	for _, entry := range menu {
		names = append(names, entry.Name)
	}
	if strings.Join(names, ",") != "The Guide,Notes,Fenced Out,plain.md,Written" {
		t.Errorf("labels = %q, want the front matter title, the first heading outside a code block, "+
			"the file name and the written label", names)
	}

	// A title changed in the document is read again.
	writeFile(t, guide, "---\ntitle: The New Guide\n---\n")
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(guide, later, later); err != nil {
		t.Fatal(err)
	}
	if menu, _ := source.resolve(handler, []render.MenuEntry{{Path: "docs/guide.md"}}); menu[0].Name != "The New Guide" {
		t.Errorf("label = %q, want the changed title", menu[0].Name)
	}
}

func TestServePageShellCarriesTheMenu(t *testing.T) {
	root := documentRoot(t)
	file := writeConfig(t, root, "menu.yaml", "menu:\n  - Home: README.md\n  - Guides:\n"+
		"      - The guide: docs/guide.md\n")
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root, watchInterval: 1,
		list: render.ListSettings{Scope: "current", Display: "shown"}, menu: &menuSource{file: file}}

	_, page := get(t, handler, "/docs/guide.md?list=scope:tree")
	for _, want := range []string{
		`<nav id="_list" class="sidebar menu">`,
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

	// The file is read again once it changes, and without a menu the list comes back.
	writeFile(t, file, "port: 9000\n")
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	if _, page := get(t, handler, "/docs/guide.md"); strings.Contains(page, "Home") ||
		!strings.Contains(page, `<a href="/docs/index.md">index.md</a>`) {
		t.Errorf("the page keeps a menu its file no longer defines: %q", page)
	}
}
