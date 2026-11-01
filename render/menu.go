package render

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// MenuEntry is one entry of a menu: the path of a document, with a label or labelled by the
// document's own title, or a label heading the entries of a group. Line is the line of the YAML
// it was read from, for the warnings naming it.
type MenuEntry struct {
	Label   string
	Path    string
	Entries []MenuEntry
	Line    int
}

// UnmarshalYAML reads an entry from a document path alone, or from a mapping of its label to a
// document path or to a sequence of the group's entries.
func (e *MenuEntry) UnmarshalYAML(node *yaml.Node) error {
	expected := fmt.Errorf("line %d: expected a menu entry, '<document>', '<label>: <document>' or "+
		"'<label>:' followed by a list of entries", node.Line)
	e.Line = node.Line
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && strings.Trim(node.Value, "/ ") != "" {
		e.Path = strings.TrimLeft(strings.TrimSpace(node.Value), "/")
		return nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return expected
	}
	key, value := node.Content[0], node.Content[1]
	if key.Kind != yaml.ScalarNode || strings.TrimSpace(key.Value) == "" {
		return fmt.Errorf("line %d: a menu entry needs a label", key.Line)
	}
	e.Label = strings.TrimSpace(key.Value)
	switch {
	case value.Kind == yaml.ScalarNode && value.Tag == "!!str" && strings.Trim(value.Value, "/ ") != "":
		e.Path = strings.TrimLeft(strings.TrimSpace(value.Value), "/")
	case value.Kind == yaml.SequenceNode && len(value.Content) > 0:
		return value.Decode(&e.Entries)
	default:
		return fmt.Errorf("line %d: '%s' needs a document path or a list of entries", value.Line, e.Label)
	}
	return nil
}

// Menu resolves menu entries to the documents of FS they name, labelled. A Menu is used by one
// goroutine at a time.
//
// Locate returns the route of the document a menu path names, and false for one that is not
// there, is a folder, or is not served. The title of a document is read again once its
// modification time changes.
type Menu struct {
	FS     fs.FS
	Locate func(name string) (route string, served bool)
	titles map[string]menuTitle
}

// menuTitle is the title a document's front matter gave it, as of the modification time read.
type menuTitle struct {
	modTime time.Time
	title   string
}

// headingPattern matches a level one ATX heading, its closing hashes left out of the text.
var headingPattern = regexp.MustCompile(`^#[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)

// fencePattern matches the line opening or closing a fenced code block.
var fencePattern = regexp.MustCompile("^ {0,3}(```|~~~)")

// title returns the label of a document named by its path alone: the title its front matter
// gives, else the text of its first level one heading outside a code block, else its file name.
func (m *Menu) title(name string) string {
	name = path.Clean(name)
	info, err := fs.Stat(m.FS, name)
	if err != nil {
		return path.Base(name)
	}
	if known, seen := m.titles[name]; seen && known.modTime.Equal(info.ModTime()) {
		return known.title
	}
	title := ""
	if text, err := fs.ReadFile(m.FS, name); err == nil {
		body := string(text)
		if match := FrontMatterPattern.FindStringSubmatchIndex(body); match != nil {
			var frontMatter map[string]any
			if yaml.Unmarshal([]byte(body[match[2]:match[3]]), &frontMatter) == nil {
				title = FrontMatterValue(frontMatter["title"])
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
		title = path.Base(name)
	}
	if m.titles == nil {
		m.titles = map[string]menuTitle{}
	}
	m.titles[name] = menuTitle{modTime: info.ModTime(), title: title}
	return title
}

// Resolve returns the entries routed to the documents they name and labelled, and a warning
// for every entry left out: a document that is not there, is a folder, or is not served, and a
// group left holding no entry.
func (m *Menu) Resolve(entries []MenuEntry) ([]ListEntry, []string) {
	menu := []ListEntry{}
	var warnings []string
	for _, entry := range entries {
		if entry.Path == "" {
			children, dropped := m.Resolve(entry.Entries)
			warnings = append(warnings, dropped...)
			if len(children) == 0 {
				warnings = append(warnings, fmt.Sprintf("line %d: the group '%s' links no document and is left out",
					entry.Line, entry.Label))
				continue
			}
			menu = append(menu, ListEntry{Name: entry.Label, Children: children})
			continue
		}
		route, served := m.Locate(entry.Path)
		if !served {
			warnings = append(warnings, fmt.Sprintf("line %d: '%s' is not a document served and is left out",
				entry.Line, entry.Path))
			continue
		}
		label := entry.Label
		if label == "" {
			label = m.title(entry.Path)
		}
		menu = append(menu, ListEntry{Name: label, Route: route})
	}
	return menu, warnings
}
