package main

import "testing"

func TestParseADOLocation(t *testing.T) {
	tests := []struct {
		location string
		want     source
		ok       bool
	}{
		{"org/proj/repo/README.md", source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/README.md"}, true},
		{"org/proj/repo/docs/guide.md", source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}, true},
		{"org/proj/repo", source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/"}, true},
		{"org/proj/repo/", source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/"}, true},
		{"org/proj/repo/docs/", source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/"}, true},
		{"my%20org/my%20proj/repo/README.md", source{kind: kindADO, organization: "my org", project: "my proj", repository: "repo", path: "/README.md"}, true},
		// An organization and a project are folders of their own, holding a listing.
		{"org/proj", source{kind: kindADO, organization: "org", project: "proj"}, true},
		{"org/proj/", source{kind: kindADO, organization: "org", project: "proj"}, true},
		{"org", source{kind: kindADO, organization: "org"}, true},
		{"org/", source{kind: kindADO, organization: "org"}, true},
		{"", source{}, false},
	}

	for _, test := range tests {
		t.Run(test.location, func(t *testing.T) {
			got, ok := parseADOLocation(test.location)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Errorf("source = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseADOURL(t *testing.T) {
	src, err := parseADOURL("ado://org/proj/repo/docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	want := source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"}
	if src != want {
		t.Errorf("source = %+v, want %+v", src, want)
	}

	// A URL naming an organization or a project addresses the listing it holds.
	for _, rawURL := range []string{"ado://org", "ado://org/proj"} {
		if _, err := parseADOURL(rawURL); err != nil {
			t.Errorf("parseADOURL(%q) raised %v", rawURL, err)
		}
	}
	if _, err := parseADOURL("ado://"); err == nil {
		t.Error("a URL naming no organization was accepted")
	}
}

func TestADOWebURLByLevel(t *testing.T) {
	tests := []struct {
		src  source
		want string
	}{
		{source{kind: kindADO, organization: "org"}, "https://dev.azure.com/org"},
		{source{kind: kindADO, organization: "org", project: "my proj"}, "https://dev.azure.com/org/my%20proj"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := adoWebURL(test.src); got != test.want {
				t.Errorf("adoWebURL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestADORouteByLevel(t *testing.T) {
	tests := []struct {
		src  source
		want string
	}{
		{source{kind: kindADO, organization: "my org"}, "/_/ado/my%20org"},
		{source{kind: kindADO, organization: "org", project: "my proj"}, "/_/ado/org/my%20proj"},
		{source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md"},
			"/_/ado/org/proj/repo/docs/guide.md"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := adoRoute(test.src, test.src.path); got != test.want {
				t.Errorf("adoRoute = %q, want %q", got, test.want)
			}
		})
	}
}

func TestADOListingDocument(t *testing.T) {
	read := listingDocument("org", "projects", []string{"Second", "first", "My Project"})
	want := "# org\n\n- [first](first)\n- [My Project](My%20Project)\n- [Second](Second)\n"
	if read.text != want {
		t.Errorf("text = %q, want %q", read.text, want)
	}
	if read.marker == "" {
		t.Error("the listing carries no change marker")
	}
	if read.name != "" {
		t.Errorf("name = %q, want empty for a listing the server wrote", read.name)
	}

	empty := listingDocument("proj", "repositories", nil)
	if want := "# proj\n\nNo repositories found.\n"; empty.text != want {
		t.Errorf("text = %q, want %q", empty.text, want)
	}
	if empty.marker == read.marker {
		t.Error("two listings carry the same change marker")
	}
}

func TestADOWebURL(t *testing.T) {
	src := source{kind: kindADO, organization: "my org", project: "proj", repository: "repo", path: "/docs/guide.md"}
	want := "https://dev.azure.com/my%20org/proj/_git/repo?path=/docs/guide.md"
	if got := adoWebURL(src); got != want {
		t.Errorf("adoWebURL = %q, want %q", got, want)
	}
}

func TestADOSourceRoute(t *testing.T) {
	handler := &server{defaultSource: source{
		kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/docs/guide.md",
	}}
	want := "/_/ado/org/proj/repo/docs/guide.md"
	if got := handler.sourceRoute(); got != want {
		t.Errorf("sourceRoute = %q, want %q", got, want)
	}
}

func TestADOTreeFolder(t *testing.T) {
	// The repository holds /docs as a folder, and nothing else is one.
	isFolder := func(src source, itemPath string) bool { return itemPath == "/docs" }

	tests := map[string]string{
		"/docs/guide.md": "/docs",
		"/docs/":         "/docs",
		"/docs":          "/docs",
		"/README.md":     "/",
		"/":              "/",
	}
	for given, want := range tests {
		t.Run(given, func(t *testing.T) {
			tree := adoTree{src: source{kind: kindADO, organization: "org", project: "proj",
				repository: "repo", path: given}, isFolder: isFolder}
			if got := tree.folder(); got != want {
				t.Errorf("folder() = %q, want %q", got, want)
			}
		})
	}
}

func TestADORoute(t *testing.T) {
	src := source{kind: kindADO, organization: "my org", project: "proj", repository: "repo"}
	tree := adoTree{src: src}
	if got, want := tree.route("/docs/my guide.md"), "/_/ado/my%20org/proj/repo/docs/my%20guide.md"; got != want {
		t.Errorf("route = %q, want %q", got, want)
	}
}

func TestADODocumentTitle(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/docs/guide.md", "guide.md"},
		{"/docs/", "docs"},
		{"/", "repo"},
	}

	handler := &server{}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			src := source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: test.path}
			if got := handler.documentTitle(src); got != test.want {
				t.Errorf("documentTitle = %q, want %q", got, test.want)
			}
		})
	}
}
