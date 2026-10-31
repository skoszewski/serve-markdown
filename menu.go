package main

import "fmt"

// readMenu returns the entries of the menu the configuration file defines, routed to the
// documents below the served directory, and a warning for every entry left out: a document
// that is not there, is a folder, or is not served, and a group left holding no entry.
func (s *server) readMenu(entries []menuEntry) ([]listEntry, []string) {
	root, _ := containedPath(s.rootDir, "")
	tree := localTree{rootDir: root}
	var menu []listEntry
	var warnings []string
	for _, entry := range entries {
		if entry.Path == "" {
			children, dropped := s.readMenu(entry.Entries)
			warnings = append(warnings, dropped...)
			if len(children) == 0 {
				warnings = append(warnings, fmt.Sprintf("line %d: the group '%s' links no document and is left out",
					entry.line, entry.Label))
				continue
			}
			menu = append(menu, listEntry{Name: entry.Label, Children: children})
			continue
		}
		file := resolveRoute("/"+entry.Path, s.rootDir, "", s.ignore)
		if file == "" || !isFile(file) {
			warnings = append(warnings, fmt.Sprintf("line %d: '%s' links '%s', which is not a document served, "+
				"and is left out", entry.line, entry.Label, entry.Path))
			continue
		}
		menu = append(menu, listEntry{Name: entry.Label, Route: tree.route(file)})
	}
	return menu, warnings
}

// menuFor returns a copy of the menu with the entry of the document src addresses marked, the
// document of a folder being the one the folder is read as.
func (s *server) menuFor(src source) []listEntry {
	route := ""
	if file := resolveRoute(src.route, s.rootDir, s.defaultFile, s.ignore); file != "" {
		if isDir(file) {
			file = s.searched().indexFile(file, s.ignore)
		}
		if root, contained := containedPath(s.rootDir, ""); contained && file != "" {
			route = localTree{rootDir: root}.route(file)
		}
	}
	return markRoute(s.menu, route)
}

// markRoute returns a copy of entries with the entries routed to route marked current.
func markRoute(entries []listEntry, route string) []listEntry {
	marked := make([]listEntry, len(entries))
	for index, entry := range entries {
		entry.Current = entry.Route != "" && entry.Route == route
		entry.Children = markRoute(entry.Children, route)
		marked[index] = entry
	}
	return marked
}
