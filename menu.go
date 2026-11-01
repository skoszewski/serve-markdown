package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skoszewski/serve-markdown/v3/render"
	"go.yaml.in/yaml/v3"
)

// menuSource is the menu of the configuration file. The file is read again once it changes,
// and the entries are resolved and labelled again on every page, a document's title read again
// once the document changes.
type menuSource struct {
	file    string
	mu      sync.Mutex
	modTime time.Time
	entries []render.MenuEntry
	// resolver resolves the entries to the documents of the served directory, keeping the
	// titles it read.
	resolver render.Menu
	// warned is what was last warned of, so a warning is logged again only once it changes.
	warned string
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
		Menu []render.MenuEntry `yaml:"menu"`
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

// resolve returns the entries routed to the documents below the served directory and
// labelled, and a warning for every entry left out: a document that is not there, is a folder,
// or is not served, and a group left holding no entry.
func (m *menuSource) resolve(s *server, entries []render.MenuEntry) ([]render.ListEntry, []string) {
	root, _ := containedPath(s.rootDir, "")
	m.resolver.FS = os.DirFS(root)
	m.resolver.Locate = func(name string) (string, bool) {
		file := resolveRoute("/"+name, s.rootDir, "", s.ignore)
		if file == "" || !isFile(file) {
			return "", false
		}
		return localTree{rootDir: root}.Route(file), true
	}
	return m.resolver.Resolve(entries)
}

// menuFor returns the menu with the entry of the document src addresses marked, the document of
// a folder being the one the folder is read as, or nil when the file defines no menu. What the
// menu is warned of is logged once each time it changes.
func (s *server) menuFor(src source) []render.ListEntry {
	m := s.menu
	m.mu.Lock()
	warnings := m.reload()
	var menu []render.ListEntry
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
			file = indexFile(s.searched(), file, s.ignore)
		}
		if root, contained := containedPath(s.rootDir, ""); contained && file != "" {
			route = localTree{rootDir: root}.Route(file)
		}
	}
	return render.MarkRoute(menu, route)
}
