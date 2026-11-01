package render

import (
	"sort"
	"strings"
)

// DefaultSearch names the documents a folder is read as, in the order they are looked for.
var DefaultSearch = IndexSearch{"README.md", "index.md"}

// IndexSearch names the documents a folder is read as, in the order they are looked for.
type IndexSearch []string

// Holds reports whether name is one of the documents a folder is read as, whatever its letters'
// case, a repository answering paths without regard to it.
func (search IndexSearch) Holds(name string) bool {
	return search.Rank(name) < len(search)
}

// Rank returns how early name stands among the documents a folder is read as, and the length of
// the search for a name that is not among them, so that the earlier of two wins.
func (search IndexSearch) Rank(name string) int {
	for rank, candidate := range search {
		if strings.EqualFold(name, candidate) {
			return rank
		}
	}
	return len(search)
}

// Named writes the documents a folder is read as, for the pages and errors that name them.
func (search IndexSearch) Named() string {
	return strings.Join(search, " or ")
}

// TreeItem is one item of a source's tree: a document to open, or a folder holding others.
type TreeItem struct {
	Path     string
	Name     string
	IsFolder bool
}

// TreeProvider reads a source as a tree of folders and documents, so that the directory list is
// built the same way whatever the source is read from.
type TreeProvider interface {
	// Root returns the folder the source itself begins at.
	Root() string
	// Folder returns the folder holding the document the page shows.
	Folder() string
	// Document returns the document the page shows, empty when the route names none.
	Document() string
	// Parent returns the folder holding folder.
	Parent(folder string) string
	// Route returns the URL route addressing the item at path.
	Route(path string) string
	// Read lists the documents and folders directly in folder, or every one below it when
	// recursive is asked for.
	Read(folder string, recursive bool) []TreeItem
}

// DocumentTree builds the entries of the directory list from provider, within scope: the
// folder the document is in ("current"), that folder and the ones below it ("subfolders"), or
// the whole tree under the source ("tree").
func DocumentTree(provider TreeProvider, scope string) []ListEntry {
	folder, recursive := provider.Folder(), false
	if scope == "tree" {
		folder, recursive = provider.Root(), true
	}

	below := map[string][]TreeItem{}
	for _, item := range provider.Read(folder, recursive) {
		parent := provider.Parent(item.Path)
		below[parent] = append(below[parent], item)
	}

	var build func(string) []ListEntry
	build = func(within string) []ListEntry {
		var folders, documents []ListEntry
		for _, item := range below[within] {
			entry := ListEntry{Name: item.Name, Route: provider.Route(item.Path)}
			if !item.IsFolder {
				entry.Current = item.Path == provider.Document()
				documents = append(documents, entry)
				continue
			}
			if scope == "current" {
				continue
			}
			if recursive {
				entry.Children = build(item.Path)
				if len(entry.Children) == 0 {
					continue
				}
			}
			folders = append(folders, entry)
		}
		SortEntries(folders)
		SortEntries(documents)
		return append(folders, documents...)
	}

	entries := build(folder)
	if scope == "subfolders" && folder != provider.Root() {
		entries = append([]ListEntry{{Name: "..", Route: provider.Route(provider.Parent(folder))}}, entries...)
	}
	return entries
}

// HoldsNothingToBrowse reports whether entries lead nowhere: they are empty, or hold the
// document the page shows and nothing else besides the ".." entry.
//
// A route naming a folder - a repository's own route among them - addresses the document
// within it without naming it, so the entry cannot be marked; an only entry named as one of the
// searched documents is that document, whatever its letters' case, since the folder resolves
// to it.
func HoldsNothingToBrowse(entries []ListEntry, search IndexSearch) bool {
	if len(entries) > 0 && entries[0].Name == ".." {
		entries = entries[1:]
	}
	if len(entries) == 0 {
		return true
	}
	if len(entries) != 1 || len(entries[0].Children) != 0 {
		return false
	}
	return entries[0].Current || search.Holds(entries[0].Name)
}

// SortEntries orders entries by name, ignoring case.
func SortEntries(entries []ListEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

// CarryQuery appends query to the route of every entry, so that browsing the list keeps the
// settings the page was opened with.
func CarryQuery(entries []ListEntry, query string) {
	if query == "" {
		return
	}
	for index := range entries {
		entries[index].Route = WithQuery(entries[index].Route, query)
		CarryQuery(entries[index].Children, query)
	}
}

// WithQuery appends query to a route, leaving a route or a query that is empty alone.
func WithQuery(route, query string) string {
	if route == "" || query == "" {
		return route
	}
	return route + "?" + query
}

// MarkRoute returns a copy of entries with the entries routed to route marked current.
func MarkRoute(entries []ListEntry, route string) []ListEntry {
	marked := make([]ListEntry, len(entries))
	for index, entry := range entries {
		entry.Current = entry.Route != "" && entry.Route == route
		entry.Children = MarkRoute(entry.Children, route)
		marked[index] = entry
	}
	return marked
}
