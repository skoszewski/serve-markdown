package main

import "testing"

func TestCarryQuery(t *testing.T) {
	entries := []listEntry{
		{Name: "docs", Route: "/docs", Children: []listEntry{{Name: "guide.md", Route: "/docs/guide.md"}}},
		{Name: "no route"},
	}
	carryQuery(entries, "list=scope:tree&outline=style:nh")

	if want := "/docs?list=scope:tree&outline=style:nh"; entries[0].Route != want {
		t.Errorf("route = %q, want %q", entries[0].Route, want)
	}
	if want := "/docs/guide.md?list=scope:tree&outline=style:nh"; entries[0].Children[0].Route != want {
		t.Errorf("the child route = %q, want %q", entries[0].Children[0].Route, want)
	}
	if entries[1].Route != "" {
		t.Errorf("an entry without a route was given %q", entries[1].Route)
	}

	carryQuery(entries, "")
	if want := "/docs?list=scope:tree&outline=style:nh"; entries[0].Route != want {
		t.Errorf("an empty query changed the route to %q", entries[0].Route)
	}
}
