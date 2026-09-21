package main

import (
	"sort"
	"strings"
)

// treeItem is one item of a source's tree: a document to open, or a folder holding others.
type treeItem struct {
	path     string
	name     string
	isFolder bool
}

// treeProvider reads a source as a tree of folders and documents, so that the directory list
// is built the same way whatever the source is read from.
type treeProvider interface {
	// root returns the folder the source itself begins at.
	root() string
	// folder returns the folder holding the document the page shows.
	folder() string
	// document returns the document the page shows, empty when the route names none.
	document() string
	// parent returns the folder holding folder.
	parent(folder string) string
	// route returns the URL route addressing the item at path.
	route(path string) string
	// read lists the documents and folders directly in folder, or every one below it when
	// recursive is asked for.
	read(folder string, recursive bool) []treeItem
}

// documentTree builds the entries of the directory list from provider, within scope: the
// folder the document is in, that folder and the ones below it, or the whole tree under the
// source.
func documentTree(provider treeProvider, scope string) []listEntry {
	folder, recursive := provider.folder(), false
	if scope == "tree" {
		folder, recursive = provider.root(), true
	}

	below := map[string][]treeItem{}
	for _, item := range provider.read(folder, recursive) {
		parent := provider.parent(item.path)
		below[parent] = append(below[parent], item)
	}

	var build func(string) []listEntry
	build = func(within string) []listEntry {
		var folders, documents []listEntry
		for _, item := range below[within] {
			entry := listEntry{Name: item.name, Route: provider.route(item.path)}
			if !item.isFolder {
				entry.Current = item.path == provider.document()
				documents = append(documents, entry)
				continue
			}
			if scope == "current" {
				continue
			}
			if recursive {
				entry.Children = build(item.path)
				if len(entry.Children) == 0 {
					continue
				}
			}
			folders = append(folders, entry)
		}
		sortEntries(folders)
		sortEntries(documents)
		return append(folders, documents...)
	}

	entries := build(folder)
	if scope == "subfolders" && folder != provider.root() {
		entries = append([]listEntry{{Name: "..", Route: provider.route(provider.parent(folder))}}, entries...)
	}
	return entries
}

// sortEntries orders entries by name, ignoring case.
func sortEntries(entries []listEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

// carryQuery appends query to the route of every entry, so that browsing the list keeps the
// settings the page was opened with.
func carryQuery(entries []listEntry, query string) {
	if query == "" {
		return
	}
	for index := range entries {
		entries[index].Route = withQuery(entries[index].Route, query)
		carryQuery(entries[index].Children, query)
	}
}

// withQuery appends query to a route, leaving a route or a query that is empty alone.
func withQuery(route, query string) string {
	if route == "" || query == "" {
		return route
	}
	return route + "?" + query
}
