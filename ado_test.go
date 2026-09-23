package main

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

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

func TestADONothingRead(t *testing.T) {
	repository := source{kind: kindADO, organization: "org", project: "proj",
		repository: "bootstrap-lz", path: "/"}
	folder := source{kind: kindADO, organization: "org", project: "proj",
		repository: "bootstrap-lz", path: "/docs/"}

	tests := map[string]struct {
		src       source
		indexOnly bool
		want      string
	}{
		// The page names what was read, the repository at its root and the folder below it.
		"a repository": {repository, false,
			"# bootstrap-lz\n\nNo README.md or index.md found here.\n"},
		"a folder": {folder, false,
			"# docs\n\nNo README.md or index.md found here.\n"},
		// Under --index-only nothing else was looked for, so the page says as much.
		"a repository read as its index alone": {repository, true,
			"# bootstrap-lz\n\nNo Markdown files found.\n"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			read := adoNothingRead(test.src, test.indexOnly, defaultSearch)
			if read.text != test.want {
				t.Errorf("text = %q, want %q", read.text, test.want)
			}
			if read.marker == "" {
				t.Error("the document carries no change marker")
			}
		})
	}
}

func TestNamedEntries(t *testing.T) {
	src := source{kind: kindADO, organization: "org", project: "proj"}
	entries := namedEntries([]string{"Second", "my repo", "first"}, "my repo", func(name string) string {
		return adoRoute(source{kind: kindADO, organization: src.organization,
			project: src.project, repository: name}, "")
	})

	want := []listEntry{
		{Name: "first", Route: "/_/ado/org/proj/first"},
		{Name: "my repo", Route: "/_/ado/org/proj/my%20repo", Current: true},
		{Name: "Second", Route: "/_/ado/org/proj/Second"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %+v, want %+v", entries, want)
	}
	for index, entry := range entries {
		if entry.Name != want[index].Name || entry.Route != want[index].Route || entry.Current != want[index].Current {
			t.Errorf("entry %d = %+v, want %+v", index, entry, want[index])
		}
	}
}

func TestADOCache(t *testing.T) {
	cache := adoCache{held: map[string]adoHeld{}}
	reads := 0
	read := func() ([]string, error) {
		reads++
		return []string{"first", "second"}, nil
	}

	names, err := cached(&cache, "projects\norg", read)
	if err != nil || len(names) != 2 || reads != 1 {
		t.Fatalf("names = %v, err = %v after %d reads", names, err, reads)
	}

	// A listing already held is not read again, and another name is read on its own.
	if _, err := cached(&cache, "projects\norg", read); err != nil || reads != 1 {
		t.Errorf("a held listing was read again: %d reads, %v", reads, err)
	}
	if _, err := cached(&cache, "projects\nother", read); err != nil || reads != 2 {
		t.Errorf("another listing was not read: %d reads, %v", reads, err)
	}

	// One that has aged out is read again.
	held := cache.held["projects\norg"]
	held.readAt = time.Now().Add(-adoTokenLifetime - time.Second)
	cache.held["projects\norg"] = held
	if _, err := cached(&cache, "projects\norg", read); err != nil || reads != 3 {
		t.Errorf("an aged listing was not read again: %d reads, %v", reads, err)
	}

	// A read that fails is not kept, and is tried again by whoever asks next.
	failing := func() ([]string, error) {
		reads++
		return nil, errors.New("cannot list")
	}
	if _, err := cached(&cache, "projects\nrefused", failing); err == nil {
		t.Error("a failing read raised no error")
	}
	if _, err := cached(&cache, "projects\nrefused", failing); err == nil || reads != 5 {
		t.Errorf("a failing read was kept: %d reads, %v", reads, err)
	}

	// What is held under one name is answered to whoever asks for that kind of thing, and
	// read afresh for another.
	repositories, err := cached(&cache, "repositories\norg\nproj", func() ([]adoRepository, error) {
		reads++
		return []adoRepository{{Name: "repo", DefaultBranch: "refs/heads/main"}}, nil
	})
	if err != nil || len(repositories) != 1 || reads != 6 {
		t.Errorf("repositories = %+v, err = %v after %d reads", repositories, err, reads)
	}
}

func TestParseADO(t *testing.T) {
	tests := map[string]adoSettings{
		"":                       {},
		"branch:main":            {Version: "main", VersionType: "branch"},
		"branch:release/2.1":     {Version: "release/2.1", VersionType: "branch"},
		"tag:v1.0":               {Version: "v1.0", VersionType: "tag"},
		"commit:9a3f2b1":         {Version: "9a3f2b1", VersionType: "commit"},
		"branch:main,branch:old": {Version: "old", VersionType: "branch"},
	}
	for given, want := range tests {
		t.Run(given, func(t *testing.T) {
			got, err := parseADO(given, adoSettings{})
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("parseADO(%q) = %+v, want %+v", given, got, want)
			}
		})
	}

	// A page's list replaces the version the server was started with, whatever kind each names.
	started := adoSettings{Version: "main", VersionType: "branch"}
	if got, err := parseADO("tag:v1.0", started); err != nil || (got != adoSettings{Version: "v1.0", VersionType: "tag"}) {
		t.Errorf("parseADO(\"tag:v1.0\", %+v) = %+v, %v", started, got, err)
	}

	for _, given := range []string{"branch:main,tag:v1.0", "tag:v1.0,commit:9a3f", "branch:", "version:main", "main"} {
		t.Run("refused "+given, func(t *testing.T) {
			if _, err := parseADO(given, adoSettings{}); err == nil {
				t.Errorf("parseADO(%q) raised no error", given)
			}
		})
	}
}

func TestADOItemsURLCarriesTheVersion(t *testing.T) {
	src := source{kind: kindADO, organization: "org", project: "my proj", repository: "repo",
		path: "/docs/guide.md", version: "release/2.1", versionType: "branch"}

	address := adoItemsURL(src, withVersion(src, url.Values{"path": []string{src.path}}))
	for _, want := range []string{
		"https://dev.azure.com/org/my%20proj/_apis/git/repositories/repo/items?",
		"versionDescriptor.version=release%2F2.1",
		"versionDescriptor.versionType=branch",
	} {
		if !strings.Contains(address, want) {
			t.Errorf("the URL does not hold %q: %q", want, address)
		}
	}

	// Without a version the repository is read at its default branch, the parameters left out.
	plain := source{kind: kindADO, organization: "org", project: "proj", repository: "repo"}
	if address := adoItemsURL(plain, withVersion(plain, url.Values{})); strings.Contains(address, "versionDescriptor") {
		t.Errorf("the URL holds a version: %q", address)
	}
}

func TestADOListLink(t *testing.T) {
	src := source{kind: kindADO, organization: "org", project: "my proj", repository: "repo"}

	tests := map[string]listLink{
		// At the repository's root the way out is the repositories of its project.
		"/":                 {Label: "Browse to repositories", Route: "/_/ado/org/my%20proj"},
		"":                  {Label: "Browse to repositories", Route: "/_/ado/org/my%20proj"},
		"/docs":             {Label: "Up", Route: "/_/ado/org/my%20proj/repo/"},
		"/docs/deep":        {Label: "Up", Route: "/_/ado/org/my%20proj/repo/docs"},
		"/docs/deep/deeper": {Label: "Up", Route: "/_/ado/org/my%20proj/repo/docs/deep"},
		"/a folder":         {Label: "Up", Route: "/_/ado/org/my%20proj/repo/"},
		"/docs/a folder":    {Label: "Up", Route: "/_/ado/org/my%20proj/repo/docs"},
	}
	for folder, want := range tests {
		t.Run(folder, func(t *testing.T) {
			if got := adoListLink(src, folder); got != want {
				t.Errorf("adoListLink(%q) = %+v, want %+v", folder, got, want)
			}
		})
	}
}

func TestADOTreeReadsOnlyTheIndexDocuments(t *testing.T) {
	src := source{kind: kindADO, organization: "org", project: "proj", repository: "repo", path: "/"}
	found := []gitItem{
		{Path: "/docs", IsFolder: true},
		{Path: "/index.md"},
		{Path: "/README.md"},
		{Path: "/CHANGELOG.md"},
		{Path: "/.hidden.md"},
		{Path: "/picture.png"},
	}

	names := func(items []treeItem) []string {
		var read []string
		for _, item := range items {
			read = append(read, item.name)
		}
		return read
	}

	all := adoTree{src: src, search: defaultSearch}.items(found, "/")
	if got := strings.Join(names(all), ","); got != "docs,index.md,README.md,CHANGELOG.md" {
		t.Errorf("items = %s, want the folders and every document", got)
	}

	// Under --index-only a document the page will never open is left out, and a folder holds
	// the one document it is read as - the README.md of an Azure Repos folder - and no other,
	// so a repository holding those two documents lists nothing to browse.
	indexed := adoTree{src: src, indexOnly: true, search: defaultSearch}.items(found, "/")
	if got := strings.Join(names(indexed), ","); got != "docs,README.md" {
		t.Errorf("items = %s, want the folders and the one document each is read as", got)
	}

	// The choice is made within each folder, the tree scope answering with all of them at once.
	nested := []gitItem{
		{Path: "/docs", IsFolder: true},
		{Path: "/docs/index.md"},
		{Path: "/docs/README.md"},
		{Path: "/docs/guide.md"},
		{Path: "/notes", IsFolder: true},
		{Path: "/notes/index.md"},
	}
	tree := adoTree{src: src, indexOnly: true, search: defaultSearch}
	held := map[string]bool{}
	for _, item := range tree.items(nested, "/") {
		held[item.path] = true
	}
	want := []string{"/docs", "/docs/README.md", "/notes", "/notes/index.md"}
	if len(held) != len(want) {
		t.Errorf("items = %v, want %v", held, want)
	}
	for _, path := range want {
		if !held[path] {
			t.Errorf("the items do not hold %q: %v", path, held)
		}
	}
}

func TestHoldsNothingToBrowse(t *testing.T) {
	tests := map[string]struct {
		entries []listEntry
		want    bool
	}{
		"the document on the page alone": {[]listEntry{{Name: "README.md", Current: true}}, true},
		"another document beside it":     {[]listEntry{{Name: "README.md", Current: true}, {Name: "notes.md"}}, false},
		"the document beside the parent": {[]listEntry{{Name: ".."}, {Name: "README.md", Current: true}}, true},
		// A repository's own route addresses the README.md within it without naming it, so the
		// entry is not marked and the name is what says it is the document on the page.
		"an unmarked README.md":        {[]listEntry{{Name: "README.md"}}, true},
		"an unmarked index.md":         {[]listEntry{{Name: "index.md"}}, true},
		"an unmarked Readme.md":        {[]listEntry{{Name: "Readme.md"}}, true},
		"one document, not the page's": {[]listEntry{{Name: "notes.md"}}, false},
		"a folder holding documents": {[]listEntry{{Name: "docs", Current: true,
			Children: []listEntry{{Name: "guide.md"}}}}, false},
		// A repository holding no Markdown at all lists nothing, which leads nowhere either.
		"nothing at all":   {nil, true},
		"an empty listing": {[]listEntry{}, true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := holdsNothingToBrowse(test.entries, defaultSearch); got != test.want {
				t.Errorf("holdsNothingToBrowse = %v, want %v", got, test.want)
			}
		})
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

func TestADOAuthorizationFromAPAT(t *testing.T) {
	// The token is carried as basic authentication under no user name.
	t.Setenv(adoPATVariable, "secret")

	got, err := adoAuthorization()
	if err != nil {
		t.Fatal(err)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte(":secret")); got != want {
		t.Errorf("adoAuthorization = %q, want %q", got, want)
	}
}
