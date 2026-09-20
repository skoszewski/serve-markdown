package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Routes under this first path segment name the kind of source to read, e.g. "/_/ado/...".
const routeNamespace = "_"

const (
	kindFile = "file"
	kindDir  = "dir"
	kindADO  = "ado"
)

const (
	dirPrefix = "dir:"
	adoScheme = "ado://"
)

var defaultCandidates = []string{"README.md", "index.md"}

const routeHelp = `Routes:
  /<path>                                              the source the server was started with
  /_/file/<path>                                       a local file under the server's directory
  /_/dir/<path>                                        a local directory's Markdown files, listed
  /_/ado/<organization>/<project>/<repository>/<path>  a file in an Azure Repos repository`

// source names one document to read: a route below the server's directory for the "file" and
// "dir" kinds, or a repository path for the "ado" kind.
type source struct {
	kind         string
	route        string
	organization string
	project      string
	repository   string
	path         string
}

// server holds what every request needs to resolve and read a document.
type server struct {
	defaultSource source
	rootDir       string
	defaultFile   string
	watchInterval float64
	assets        assetURLs
	online        bool
}

// unescapePath decodes the percent escapes in a URL path, leaving it unchanged when it holds
// an invalid escape.
func unescapePath(path string) string {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return decoded
}

// containedPath joins relative onto rootDir and returns the result only when it stays inside
// rootDir, so a route cannot reach a file the server was not pointed at.
func containedPath(rootDir, relative string) (string, bool) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", false
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = resolved
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}

// isFile reports whether path exists as a regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// isDir reports whether path exists as a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// findIndexFile returns the first of defaultCandidates that exists as a file under directory,
// or "" when none does.
func findIndexFile(directory string) string {
	for _, candidate := range defaultCandidates {
		path := filepath.Join(directory, candidate)
		if isFile(path) {
			return path
		}
	}
	return ""
}

// resolveRoute resolves a URL route to a file under rootDir, or "" if it cannot be served.
//
// defaultFile is the file the "/" route serves; without one the defaultCandidates are tried.
func resolveRoute(route, rootDir, defaultFile string) string {
	relative := strings.TrimLeft(unescapePath(route), "/")
	if relative == "" {
		if defaultFile != "" {
			return defaultFile
		}
		return findIndexFile(rootDir)
	}

	resolved, contained := containedPath(rootDir, relative)
	if !contained {
		return ""
	}
	if isDir(resolved) {
		return findIndexFile(resolved)
	}
	if isFile(resolved) {
		return resolved
	}
	return ""
}

// resolveDirRoute resolves a URL route to a file or directory under rootDir, or "" if neither
// exists.
//
// Unlike resolveRoute, a directory is returned as itself rather than resolved to its index
// file, since the "dir" source lists the directory's Markdown files instead of requiring one
// of them to be named README.md or index.md.
func resolveDirRoute(route, rootDir string) string {
	relative := strings.TrimLeft(unescapePath(route), "/")
	resolved, contained := containedPath(rootDir, relative)
	if !contained {
		return ""
	}
	if isDir(resolved) || isFile(resolved) {
		return resolved
	}
	return ""
}

// renderDirectoryListing returns a Markdown document listing the Markdown files found
// directly under directory.
//
// Subdirectories are not scanned; a link to a file below one still works, since it is served
// the way any other file under the "dir" source is.
func renderDirectoryListing(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !markdownExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	lines := []string{"# " + filepath.Base(directory), ""}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- [%s](%s)", name, name))
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// readLocalFile reads a local file and returns it as (change marker, Markdown text, CSS).
func readLocalFile(path string) (string, string, string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", err
	}
	css, markdown := asMarkdownDocument(filepath.Base(path), string(text))
	return strconv.FormatInt(info.ModTime().UnixNano(), 10), markdown, css, nil
}

// resolveSource resolves a URL route to the document source it addresses.
//
// A route under routeNamespace names the kind of source to read from - "/_/file/<path>",
// "/_/dir/<path>" or "/_/ado/<organization>/<project>/<repository>/<path>" - which is served
// whatever the server was started with. Any other route is read from the source the server
// was started with, addressing a document below it.
//
// The second result is false when the route names a scheme but no document within it.
func resolveSource(route string, defaultSource source) (source, bool) {
	namespace, remainder, _ := strings.Cut(strings.TrimLeft(route, "/"), "/")
	if namespace == routeNamespace {
		scheme, rest, _ := strings.Cut(remainder, "/")
		switch scheme {
		case kindFile, kindDir:
			return source{kind: scheme, route: "/" + rest}, true
		case kindADO:
			return parseADOLocation(rest)
		}
		return source{}, false
	}

	if defaultSource.kind == kindFile || defaultSource.kind == kindDir {
		return source{kind: defaultSource.kind, route: route}, true
	}

	relative := strings.TrimLeft(unescapePath(route), "/")
	if relative == "" {
		return defaultSource, true
	}
	addressed := defaultSource
	addressed.path = "/" + relative
	return addressed, true
}

// sourceRoute returns the URL route that addresses source, for the server to print at startup.
func (s *server) sourceRoute() string {
	if s.defaultSource.kind == kindADO {
		return fmt.Sprintf("/%s/ado/%s/%s/%s%s", routeNamespace, s.defaultSource.organization,
			s.defaultSource.project, s.defaultSource.repository, s.defaultSource.path)
	}
	if s.defaultFile != "" {
		relative, err := filepath.Rel(s.rootDir, s.defaultFile)
		if err == nil {
			return fmt.Sprintf("/%s/file/%s", routeNamespace, filepath.ToSlash(relative))
		}
	}
	return "/"
}

// documentTitle returns the page title for source, or "" when it addresses no document.
func (s *server) documentTitle(src source) string {
	switch src.kind {
	case kindADO:
		name := strings.TrimSuffix(src.path, "/")
		if index := strings.LastIndex(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		if name == "" {
			return src.repository
		}
		return name
	case kindDir:
		path := resolveDirRoute(src.route, s.rootDir)
		if path == "" {
			return ""
		}
		return filepath.Base(path)
	}

	path := resolveRoute(src.route, s.rootDir, s.defaultFile)
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// loadDocument reads the document source addresses and returns it as (change marker, Markdown
// text, CSS).
//
// The change marker identifies the version that was read - a modification time, a Git object
// ID - so that the page re-renders only once it differs from the one it holds.
func (s *server) loadDocument(src source) (string, string, string, error) {
	switch src.kind {
	case kindADO:
		itemPath, text, objectID, err := readADODocument(src)
		if err != nil {
			return "", "", "", err
		}
		name := itemPath
		if index := strings.LastIndex(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		css, markdown := asMarkdownDocument(name, text)
		return objectID, markdown, css, nil

	case kindDir:
		path := resolveDirRoute(src.route, s.rootDir)
		if path == "" {
			return "", "", "", fmt.Errorf("no such file or directory for route '%s'", src.route)
		}
		if isDir(path) {
			info, err := os.Stat(path)
			if err != nil {
				return "", "", "", err
			}
			listing, err := renderDirectoryListing(path)
			if err != nil {
				return "", "", "", err
			}
			return strconv.FormatInt(info.ModTime().UnixNano(), 10), listing, "", nil
		}
		return readLocalFile(path)
	}

	path := resolveRoute(src.route, s.rootDir, s.defaultFile)
	if path == "" {
		return "", "", "", fmt.Errorf("no such file for route '%s'", src.route)
	}
	return readLocalFile(path)
}
