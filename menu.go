package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// menuSource is the menu of the configuration file. The file is read again once it changes,
// and the entries are resolved and labelled again on every page, a document's title read again
// once the document changes.
type menuSource struct {
	file    string
	mu      sync.Mutex
	modTime time.Time
	entries []menuEntry
	titles  map[string]menuTitle
	// warned is what was last warned of, so a warning is logged again only once it changes.
	warned string
}

// menuTitle is the title a document's front matter gave it, as of the modification time read.
type menuTitle struct {
	modTime time.Time
	title   string
}

// reload reads the menu again once its file has changed, keeping the menu it had, with a
// warning, when the file no longer reads.
func (m *menuSource) reload() []string {
	if m.file == "" {
		return nil
	}
	info, err := os.Stat(m.file)
	if err != nil {
		return []string{fmt.Sprintf("cannot read '%s' (%v); the menu stays as it was", m.file, err)}
	}
	if info.ModTime().Equal(m.modTime) {
		return nil
	}
	content, err := os.ReadFile(m.file)
	var read struct {
		Menu []menuEntry `yaml:"menu"`
	}
	if err == nil {
		err = yaml.Unmarshal(content, &read)
	}
	if err != nil {
		return []string{fmt.Sprintf("cannot read the menu of '%s' (%s); the menu stays as it was",
			m.file, configError(err))}
	}
	m.modTime, m.entries = info.ModTime(), read.Menu
	return nil
}

// headingPattern matches a level one ATX heading, its closing hashes left out of the text.
var headingPattern = regexp.MustCompile(`^#[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)

// fencePattern matches the line opening or closing a fenced code block.
var fencePattern = regexp.MustCompile("^ {0,3}(```|~~~)")

// title returns the label of a document named by its path alone: the title its front matter
// gives, else the text of its first level one heading outside a code block, else its file name.
func (m *menuSource) title(file string) string {
	info, err := os.Stat(file)
	if err != nil {
		return filepath.Base(file)
	}
	if known, seen := m.titles[file]; seen && known.modTime.Equal(info.ModTime()) {
		return known.title
	}
	title := ""
	if text, err := os.ReadFile(file); err == nil {
		body := string(text)
		if match := frontMatterPattern.FindStringSubmatchIndex(body); match != nil {
			var frontMatter map[string]any
			if yaml.Unmarshal([]byte(body[match[2]:match[3]]), &frontMatter) == nil {
				title = frontMatterValue(frontMatter["title"])
			}
			body = body[match[1]:]
		}
		fence := ""
		for _, line := range strings.Split(body, "\n") {
			if title != "" {
				break
			}
			line = strings.TrimRight(line, "\r")
			if marker := fencePattern.FindStringSubmatch(line); marker != nil {
				if fence == "" {
					fence = marker[1]
				} else if marker[1] == fence {
					fence = ""
				}
				continue
			}
			if heading := headingPattern.FindStringSubmatch(line); fence == "" && heading != nil {
				title = strings.TrimSpace(heading[1])
			}
		}
	}
	if title == "" {
		title = filepath.Base(file)
	}
	if m.titles == nil {
		m.titles = map[string]menuTitle{}
	}
	m.titles[file] = menuTitle{modTime: info.ModTime(), title: title}
	return title
}

// resolve returns the entries routed to the documents below the served directory and
// labelled, and a warning for every entry left out: a document that is not there, is a folder,
// or is not served, and a group left holding no entry.
func (m *menuSource) resolve(s *server, entries []menuEntry) ([]listEntry, []string) {
	root, _ := containedPath(s.rootDir, "")
	tree := localTree{rootDir: root}
	menu := []listEntry{}
	var warnings []string
	for _, entry := range entries {
		if entry.Path == "" {
			children, dropped := m.resolve(s, entry.Entries)
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
			warnings = append(warnings, fmt.Sprintf("line %d: '%s' is not a document served and is left out",
				entry.line, entry.Path))
			continue
		}
		label := entry.Label
		if label == "" {
			label = m.title(file)
		}
		menu = append(menu, listEntry{Name: label, Route: tree.route(file)})
	}
	return menu, warnings
}

// menuFor returns the menu with the entry of the document src addresses marked, the document of
// a folder being the one the folder is read as, or nil when the file defines no menu. What the
// menu is warned of is logged once each time it changes.
func (s *server) menuFor(src source) []listEntry {
	m := s.menu
	m.mu.Lock()
	warnings := m.reload()
	var menu []listEntry
	if len(m.entries) > 0 {
		var dropped []string
		menu, dropped = m.resolve(s, m.entries)
		warnings = append(warnings, dropped...)
	}
	if joined := strings.Join(warnings, "\n"); joined != m.warned {
		m.warned = joined
		for _, warning := range warnings {
			logInfo("  Menu:     %sWarning: %s%s", colorYellow, warning, colorReset)
		}
	}
	m.mu.Unlock()
	if menu == nil {
		return nil
	}

	route := ""
	if file := resolveRoute(src.route, s.rootDir, s.defaultFile, s.ignore); file != "" {
		if isDir(file) {
			file = s.searched().indexFile(file, s.ignore)
		}
		if root, contained := containedPath(s.rootDir, ""); contained && file != "" {
			route = localTree{rootDir: root}.route(file)
		}
	}
	return markRoute(menu, route)
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
