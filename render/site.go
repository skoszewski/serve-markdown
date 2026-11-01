package render

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options is how a Site draws its pages.
//
// Prefix is the route the Site is served under, ending in a slash, "/" when empty. Assets holds
// the built browser files, as AssetHandler takes them, and Banner the text beside the control
// buttons. Search names the documents a folder is read as, DefaultSearch when empty. Menu stands
// in place of the directory list when it holds entries. Outline, List, ContentWidth and
// Separators draw the page as PageSettings does, and Mermaid draws diagrams when Assets holds
// mermaid.min.js. CSS styles every document whose front matter declares none, and
// WatchInterval is how often a page fetches its document again, never when zero.
type Options struct {
	Prefix        string
	Assets        fs.FS
	Banner        string
	Search        IndexSearch
	Menu          []MenuEntry
	Outline       OutlineSettings
	List          ListSettings
	ContentWidth  string
	Separators    string
	Mermaid       bool
	CSS           string
	WatchInterval time.Duration
}

// Site serves the Markdown documents of an fs.FS as pages, under the route prefix of its
// options.
//
// Below the prefix, "_/content" answers the page's fetch of its document, "_/assets/" and
// "_/page/" serve the assets, a route naming a picture or another binary file is answered with
// its bytes, and any other route naming a document or a folder with the page shell drawing it.
// A folder is read as the first document of the search within it, else as the listing of the
// Markdown files it holds. Names beginning with a dot are neither listed nor served.
type Site struct {
	files   fs.FS
	options Options
	assets  http.Handler
	mermaid bool
	mu      sync.Mutex
	menu    Menu
}

// New returns the Site serving the documents of files.
func New(files fs.FS, options Options) *Site {
	if options.Prefix == "" {
		options.Prefix = "/"
	}
	if len(options.Search) == 0 {
		options.Search = DefaultSearch
	}
	site := &Site{files: files, options: options, assets: AssetHandler(options.Prefix+"_/", options.Assets)}
	if options.Mermaid && options.Assets != nil {
		_, err := fs.Stat(options.Assets, "mermaid.min.js")
		site.mermaid = err == nil
	}
	site.menu = Menu{FS: files, Locate: func(name string) (string, bool) {
		name = path.Clean(name)
		if !site.served(name) {
			return "", false
		}
		info, err := fs.Stat(files, name)
		if err != nil || !info.Mode().IsRegular() {
			return "", false
		}
		return site.route(name), true
	}}
	return site
}

// served reports whether name is a valid path of the files without a segment beginning with a
// dot.
func (s *Site) served(name string) bool {
	if !fs.ValidPath(name) {
		return false
	}
	return name == "." || !slices.ContainsFunc(strings.Split(name, "/"), func(segment string) bool {
		return strings.HasPrefix(segment, ".")
	})
}

// route returns the route addressing the file or folder name.
func (s *Site) route(name string) string {
	if name == "." {
		return s.options.Prefix
	}
	return s.options.Prefix + EscapeRoute(name)
}

// resolve returns the name of the file or folder route addresses, and whether it is a folder;
// the name is empty for a route reaching nothing that is served.
func (s *Site) resolve(route string) (string, bool) {
	relative, below := strings.CutPrefix(route, s.options.Prefix)
	switch {
	case !below && route+"/" == s.options.Prefix:
		relative = ""
	case !below:
		return "", false
	}
	name := strings.Trim(path.Clean("/"+UnescapePath(relative)), "/")
	if name == "" {
		name = "."
	}
	if !s.served(name) {
		return "", false
	}
	info, err := fs.Stat(s.files, name)
	if err != nil {
		return "", false
	}
	return name, info.IsDir()
}

// indexFile returns the first document of the search within folder, or "" when it holds none.
func (s *Site) indexFile(folder string) string {
	for _, candidate := range s.options.Search {
		name := path.Join(folder, candidate)
		if info, err := fs.Stat(s.files, name); err == nil && info.Mode().IsRegular() {
			return name
		}
	}
	return ""
}

// ServeHTTP answers a request for a route below the prefix.
func (s *Site) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	internal := s.options.Prefix + "_/"
	switch {
	case request.URL.Path == internal+"content":
		s.sendContent(writer, request)
		return
	case strings.HasPrefix(request.URL.Path, internal):
		s.assets.ServeHTTP(writer, request)
		return
	}

	name, folder := s.resolve(request.URL.Path)
	if name == "" {
		http.NotFound(writer, request)
		return
	}
	if !folder && IsRawAsset(name) {
		writer.Header().Set("Content-Security-Policy", "sandbox")
		http.ServeFileFS(writer, request, s.files, name)
		return
	}

	// The page's own document, the one a folder is read as, is its title and its menu entry.
	document := name
	if folder {
		if index := s.indexFile(name); index != "" {
			document = index
		}
	}
	title := path.Base(document)
	if document == "." {
		title = path.Base(strings.TrimSuffix(s.options.Prefix, "/"))
	}

	beside := Sidebars{Outline: s.options.Outline, List: s.options.List}
	if len(s.options.Menu) > 0 {
		s.mu.Lock()
		menu, _ := s.menu.Resolve(s.options.Menu)
		s.mu.Unlock()
		beside.Entries, beside.Menu = MarkRoute(menu, s.route(document)), true
	} else if beside.List.Scope != "" {
		tree := siteTree{site: s, folder: name}
		if !folder {
			tree.folder, tree.document = path.Dir(name), name
		}
		beside.Entries = DocumentTree(tree, beside.List.Scope)
	}
	if beside.HoldsList() {
		beside.Outline.Justify = "right"
	}

	page := Page(title, internal+"content?path="+url.QueryEscape(request.URL.Path),
		int(s.options.WatchInterval/time.Millisecond), PageSettings{AssetPrefix: internal,
			Banner: s.options.Banner, Sidebars: beside, ContentWidth: s.options.ContentWidth,
			Separators: s.options.Separators, Mermaid: s.mermaid})
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Write(page)
}

// sendContent answers the page's fetch of the document the route in its "path" parameter
// addresses, or with the error that reading it raised.
func (s *Site) sendContent(writer http.ResponseWriter, request *http.Request) {
	content := Content{}
	route := request.URL.Query().Get("path")
	fail := func(message string) {
		content.Error = &message
	}

	name, folder := s.resolve(route)
	read, marker, document := "", "", ""
	switch {
	case name == "":
		fail("'" + route + "' names no document")
	case folder && s.indexFile(name) == "":
		var names []string
		entries, err := fs.ReadDir(s.files, name)
		if err != nil {
			fail(err.Error())
			break
		}
		for _, entry := range entries {
			if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") &&
				MarkdownExtensions[strings.ToLower(path.Ext(entry.Name()))] {
				names = append(names, entry.Name())
			}
		}
		sort.SliceStable(names, func(i, j int) bool {
			return strings.ToLower(names[i]) < strings.ToLower(names[j])
		})
		title := path.Base(name)
		if name == "." {
			title = path.Base(strings.TrimSuffix(s.options.Prefix, "/"))
		}
		read = DirectoryListing(title, names)
		marker = TextMarker(read)
	default:
		document = name
		if folder {
			document = s.indexFile(name)
		}
		text, err := fs.ReadFile(s.files, document)
		if err != nil {
			fail(err.Error())
			break
		}
		read = string(text)
		marker = TextMarker(read)
		if info, err := fs.Stat(s.files, document); err == nil && !info.ModTime().IsZero() {
			marker = strconv.FormatInt(info.ModTime().UnixNano(), 10)
		}
	}

	if content.Error == nil {
		css, info, markdown, documentName := "", DocumentInfo{}, read, ""
		if document != "" {
			documentName = path.Base(document)
			if MarkdownExtensions[strings.ToLower(path.Ext(document))] {
				css, info, markdown, _ = FrontMatter(read)
			} else {
				markdown = CodeDocument(documentName, read)
			}
		}
		if css == "" {
			css = s.options.CSS
		}
		base := DocumentBase(route, documentName)
		content.MTime, content.Text, content.CSS, content.Base, content.Info = &marker, &markdown, &css, &base, &info
	}

	answer, err := json.Marshal(content)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write(answer)
}

// siteTree reads the files of a Site as the tree the directory list is built from, by their
// names in the files.
type siteTree struct {
	site     *Site
	folder   string
	document string
}

func (t siteTree) Root() string     { return "." }
func (t siteTree) Folder() string   { return t.folder }
func (t siteTree) Document() string { return t.document }

func (t siteTree) Parent(folder string) string { return path.Dir(folder) }

func (t siteTree) Route(name string) string { return t.site.route(name) }

func (t siteTree) Read(folder string, recursive bool) []TreeItem {
	var items []TreeItem
	fs.WalkDir(t.site.files, folder, func(name string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil, name == folder:
			return nil
		case strings.HasPrefix(entry.Name(), "."):
			if entry.IsDir() {
				return fs.SkipDir
			}
		case entry.IsDir():
			items = append(items, TreeItem{Path: name, Name: entry.Name(), IsFolder: true})
			if !recursive {
				return fs.SkipDir
			}
		case MarkdownExtensions[strings.ToLower(path.Ext(entry.Name()))]:
			items = append(items, TreeItem{Path: name, Name: entry.Name()})
		}
		return nil
	})
	return items
}
