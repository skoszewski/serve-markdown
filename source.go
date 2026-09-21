package main

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Routes under this first path segment name the kind of source to read, e.g. "/_/ado/...".
const routeNamespace = "_"

const (
	kindLocal = "local"
	kindADO   = "ado"
)

const adoScheme = "ado://"

// A folder is read as the first of these documents it holds: a local directory names its
// index first, an Azure Repos folder its README, as each is usually written.
var (
	localCandidates = []string{"index.md", "README.md"}
	adoCandidates   = []string{"README.md", "index.md"}
)

// indexNames are the names a folder's own document goes by, whichever source it is read from.
var indexNames = []string{"index.md", "README.md"}

const routeHelp = `Routes:
  /<path>                                              a local file or directory under the server's directory
  /_/ado/<organization>                                the projects of an Azure DevOps organization
  /_/ado/<organization>/<project>                      the Git repositories of a project
  /_/ado/<organization>/<project>/<repository><path>   a file in an Azure Repos repository

A directory resolves to the README.md or index.md within it, and is listed when it holds
neither; an organization and a project are always listed.`

// source names one document to read: a route below the server's directory for the "local"
// kind, or a repository path for the "ado" kind.
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
	outline       outlineSettings
	list          listSettings
	mermaid       bool
	indexOnly     bool
}

// Files with these suffixes are served as the bytes they hold rather than read as documents.
var rawAssetExtensions = map[string]bool{
	".avif": true, ".bmp": true, ".gif": true, ".ico": true, ".jpeg": true,
	".jpg": true, ".pdf": true, ".png": true, ".svg": true, ".webp": true,
}

// isRawAsset reports whether path names a file served as its own bytes.
func isRawAsset(path string) bool {
	return rawAssetExtensions[strings.ToLower(filepath.Ext(path))]
}

// contentTypeFor returns the media type path's suffix names, falling back to a byte stream.
func contentTypeFor(path string) string {
	if mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
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

// findIndexFile returns the first of localCandidates that exists as a file under directory,
// or "" when none does.
func findIndexFile(directory string) string {
	for _, candidate := range localCandidates {
		path := filepath.Join(directory, candidate)
		if isFile(path) {
			return path
		}
	}
	return ""
}

// resolveRoute resolves a URL route to the file or directory under rootDir it addresses, or
// "" when it reaches neither.
//
// defaultFile is the file the "/" route serves; without one the route reads rootDir itself.
// A directory is returned as itself, and resolved to the document within it when one is read.
func resolveRoute(route, rootDir, defaultFile string) string {
	relative := strings.TrimLeft(unescapePath(route), "/")
	if relative == "" && defaultFile != "" {
		return defaultFile
	}

	resolved, contained := containedPath(rootDir, relative)
	if !contained {
		return ""
	}
	if isDir(resolved) || isFile(resolved) {
		return resolved
	}
	return ""
}

// markdownFilesIn returns the names of the Markdown files directly under directory, ordered
// by name.
//
// Subdirectories are not scanned; the directory list beside the page reaches those.
func markdownFilesIn(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
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
	return names, nil
}

// renderDirectoryListing returns the Markdown document listing names as the files directory
// holds, or saying that it holds none.
//
// It is the document of a directory holding neither index.md nor README.md; --index-only asks
// for it with no names at all, having stopped looking for the rest.
func renderDirectoryListing(directory string, names []string) string {
	lines := []string{"# " + filepath.Base(directory), ""}
	if len(names) == 0 {
		lines = append(lines, "No Markdown files found.")
	}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- [%s](%s)", name, name))
	}
	return strings.Join(lines, "\n") + "\n"
}

// readLocalFile reads a local file as the document it holds.
func readLocalFile(path string) (document, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return document{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return document{}, err
	}
	name := filepath.Base(path)
	css, markdown := asMarkdownDocument(name, string(text))
	return document{marker: strconv.FormatInt(info.ModTime().UnixNano(), 10),
		name: name, text: markdown, css: css}, nil
}

// resolveSource resolves a URL route to the document source it addresses.
//
// A route under routeNamespace reads an Azure Repos repository -
// "/_/ado/<organization>/<project>/<repository><path>" - whatever the server was started
// with, since the repository is named by the route itself. Every other route is local,
// addressing a file or a directory below the server's own directory.
//
// The second result is false when the route names no document.
func resolveSource(route string, defaultSource source) (source, bool) {
	namespace, remainder, _ := strings.Cut(strings.TrimLeft(route, "/"), "/")
	if namespace == routeNamespace {
		if scheme, rest, _ := strings.Cut(remainder, "/"); scheme == kindADO {
			return parseADOLocation(rest)
		}
		return source{}, false
	}
	return source{kind: kindLocal, route: route}, true
}

// escapeRoute escapes each segment of a slash separated path, for a route that addresses it.
func escapeRoute(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// documentList returns the directory list for the document src addresses: the entries beside
// it, and the link leading out of them.
func (s *server) documentList(src source, scope string) ([]listEntry, listLink) {
	switch src.kind {
	case kindLocal:
		resolved := resolveRoute(src.route, s.rootDir, s.defaultFile)
		root, contained := containedPath(s.rootDir, "")
		if resolved == "" || !contained {
			return nil, listLink{}
		}
		tree := localTree{rootDir: root, folderPath: resolved}
		if isFile(resolved) {
			tree.documentPath = resolved
			tree.folderPath = filepath.Dir(resolved)
		}
		return documentTree(tree, scope), listLink{}

	case kindADO:
		return adoList(src, scope)
	}
	return nil, listLink{}
}

// localTree reads a local directory as the tree the directory list is built from.
type localTree struct {
	rootDir      string
	folderPath   string
	documentPath string
}

func (t localTree) root() string     { return t.rootDir }
func (t localTree) folder() string   { return t.folderPath }
func (t localTree) document() string { return t.documentPath }

func (t localTree) parent(folder string) string { return filepath.Dir(folder) }

// route addresses a local path as the route below the server's directory that reaches it.
func (t localTree) route(path string) string {
	relative, err := filepath.Rel(t.rootDir, path)
	if err != nil {
		return ""
	}
	if relative == "." {
		relative = ""
	}
	return "/" + escapeRoute(filepath.ToSlash(relative))
}

func (t localTree) read(folder string, recursive bool) []treeItem {
	var items []treeItem
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case path == folder:
			return nil
		case strings.HasPrefix(entry.Name(), "."):
			if entry.IsDir() {
				return fs.SkipDir
			}
		case entry.IsDir():
			items = append(items, treeItem{path: path, name: entry.Name(), isFolder: true})
			if !recursive {
				return fs.SkipDir
			}
		case markdownExtensions[strings.ToLower(filepath.Ext(entry.Name()))]:
			items = append(items, treeItem{path: path, name: entry.Name()})
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return items
}

// sendRawAsset answers with the bytes of the picture or other binary file src addresses,
// sandboxed.
func (s *server) sendRawAsset(writer http.ResponseWriter, request *http.Request, src source) {
	writer.Header().Set("Content-Security-Policy", "sandbox")

	if src.kind == kindADO {
		content, err := readADORawItem(src)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", contentTypeFor(src.path))
		writer.Write(content)
		return
	}

	path := resolveRoute(src.route, s.rootDir, s.defaultFile)
	if path == "" || !isFile(path) {
		http.Error(writer, fmt.Sprintf("no such file for route '%s'", src.route), http.StatusNotFound)
		return
	}

	// The file is opened and served itself rather than through http.ServeFile, which refuses
	// any route holding '..' - one a document may well link a picture by, and one the route
	// has already been resolved through.
	file, err := os.Open(path)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	http.ServeContent(writer, request, filepath.Base(path), info.ModTime(), file)
}

// sourceRoute returns the URL route that addresses source, for the server to print at startup.
func (s *server) sourceRoute() string {
	if s.defaultSource.kind == kindADO {
		return adoRoute(s.defaultSource, s.defaultSource.path)
	}
	return "/"
}

// documentTitle returns the page title for source, or "" when it addresses no document.
func (s *server) documentTitle(src source) string {
	switch src.kind {
	case kindADO:
		if src.project == "" {
			return src.organization
		}
		if src.repository == "" {
			return src.project
		}
		name := strings.TrimSuffix(src.path, "/")
		if index := strings.LastIndex(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		if name == "" {
			return src.repository
		}
		return name
	}

	path := resolveRoute(src.route, s.rootDir, s.defaultFile)
	if path == "" {
		return ""
	}
	if index := findIndexFile(path); isDir(path) && index != "" {
		return filepath.Base(index)
	}
	return filepath.Base(path)
}

// document is one document read from a source.
//
// The marker identifies the version that was read - a modification time, a Git object ID - so
// that the page re-renders only once it differs from the one it holds. The name is the file
// the document was read from, which a route naming a folder resolves to, and is empty for a
// listing the server wrote itself.
type document struct {
	marker string
	name   string
	text   string
	css    string
}

// loadDocument reads the document source addresses.
//
// A directory is the index.md or README.md within it; holding neither, it is the listing of
// the Markdown files it does hold, which --index-only leaves out, having stopped looking.
func (s *server) loadDocument(src source) (document, error) {
	if src.kind == kindADO {
		return readADOSource(src, s.indexOnly)
	}

	path := resolveRoute(src.route, s.rootDir, s.defaultFile)
	if path == "" {
		return document{}, fmt.Errorf("no such file or directory for route '%s'", src.route)
	}
	if !isDir(path) {
		return readLocalFile(path)
	}
	if index := findIndexFile(path); index != "" {
		return readLocalFile(index)
	}

	info, err := os.Stat(path)
	if err != nil {
		return document{}, err
	}
	var names []string
	if !s.indexOnly {
		if names, err = markdownFilesIn(path); err != nil {
			return document{}, err
		}
	}
	return document{marker: strconv.FormatInt(info.ModTime().UnixNano(), 10),
		text: renderDirectoryListing(path, names)}, nil
}

// documentBase returns the route relative links in the document are resolved against: the
// route of the folder holding it, ending in a slash.
//
// name is the file the document was read from; a route ending in it names the document
// itself, and any other route names the folder it lies in.
func documentBase(route, name string) string {
	if route == "" {
		route = "/"
	}
	if name != "" && unescapePath(path.Base(route)) == name {
		route = path.Dir(route)
	}
	return strings.TrimSuffix(route, "/") + "/"
}

// textMarker returns the change marker of a document the server wrote itself, which has no
// modification time or object ID of its own.
func textMarker(text string) string {
	sum := fnv.New64a()
	sum.Write([]byte(text))
	return strconv.FormatUint(sum.Sum64(), 16)
}
