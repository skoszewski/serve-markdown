package render

import "testing"

func TestCarryQuery(t *testing.T) {
	entries := []ListEntry{
		{Name: "docs", Route: "/docs", Children: []ListEntry{{Name: "guide.md", Route: "/docs/guide.md"}}},
		{Name: "no route"},
	}
	CarryQuery(entries, "list=scope:tree&outline=style:nh")

	if want := "/docs?list=scope:tree&outline=style:nh"; entries[0].Route != want {
		t.Errorf("route = %q, want %q", entries[0].Route, want)
	}
	if want := "/docs/guide.md?list=scope:tree&outline=style:nh"; entries[0].Children[0].Route != want {
		t.Errorf("the child route = %q, want %q", entries[0].Children[0].Route, want)
	}
	if entries[1].Route != "" {
		t.Errorf("an entry without a route was given %q", entries[1].Route)
	}

	CarryQuery(entries, "")
	if want := "/docs?list=scope:tree&outline=style:nh"; entries[0].Route != want {
		t.Errorf("an empty query changed the route to %q", entries[0].Route)
	}
}

func TestIndexSearch(t *testing.T) {
	search := IndexSearch{"README.md", "index.md"}

	if !search.Holds("readme.md") || !search.Holds("INDEX.MD") {
		t.Error("a document of the search is not held, its letters' case aside")
	}
	if search.Holds("guide.md") {
		t.Error("a document the search does not name is held")
	}
	if search.Rank("README.md") >= search.Rank("index.md") {
		t.Error("the search does not rank its documents in the order they are looked for")
	}
	if search.Rank("guide.md") != len(search) {
		t.Errorf("rank of a document outside the search = %d, want %d", search.Rank("guide.md"), len(search))
	}
	if want := "README.md or index.md"; search.Named() != want {
		t.Errorf("Named = %q, want %q", search.Named(), want)
	}
}
