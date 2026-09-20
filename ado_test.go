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
		{"org/proj", source{}, false},
		{"org", source{}, false},
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

	if _, err := parseADOURL("ado://org/proj"); err == nil {
		t.Error("an incomplete URL was accepted")
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
