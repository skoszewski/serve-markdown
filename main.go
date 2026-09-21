package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddress  = "127.0.0.1"
	defaultServePort      = 8000
	defaultWatchInterval  = 1.0
	defaultADOWatchSecond = 15.0
)

const pathUsage = "Markdown file or directory to serve; defaults to the current directory. " +
	"A directory is served at its own URL path, resolving to index.md or README.md within it " +
	"and listing its Markdown files when it holds neither. An " +
	"'ado://<organization>/<project>/<repository>/<path to file>' URL serves a file from an " +
	"Azure Repos Git repository, under its own /_/ado/ route rather than at the root. " +
	"Whichever is given, the /_/local/ and /_/ado/ routes reach both while the server runs."

// outlineStyles are the values --outline takes, each with the shorthand that also names it:
// an outline without numbers, one numbered within each level, and one numbered as 1., 1.1.,
// 1.1.1. down the levels.
var outlineStyles = []struct{ name, shorthand string }{
	{"plain", "p"},
	{"numbered", "n"},
	{"numbered-hierarchical", "nh"},
}

// outlineNone is the value that asks for no outline at all.
const outlineNone = "none"

// outlineJustifications are the sides of the document the outline stands on.
var outlineJustifications = []string{"left", "right"}

// defaultOutline is what an outline is drawn with before --outline or the query names
// anything else.
var defaultOutline = outlineSettings{Style: "plain", Justify: "left"}

// outlineStyle returns the style given names, by its name, its shorthand or outlineNone for
// no outline. The second result is false when it names none of them.
func outlineStyle(given string) (string, bool) {
	if given == outlineNone {
		return "", true
	}
	for _, style := range outlineStyles {
		if given == style.name || given == style.shorthand {
			return style.name, true
		}
	}
	return "", false
}

// outlineStyleList names the styles with their shorthands, for the flag's help and its errors.
func outlineStyleList() string {
	names := make([]string, 0, len(outlineStyles)+1)
	for _, style := range outlineStyles {
		names = append(names, fmt.Sprintf("%s (%s)", style.name, style.shorthand))
	}
	return strings.Join(append(names, outlineNone), ", ")
}

// parseOutline reads the "style" and "justify" settings onto the ones it is given, and
// returns the outline the list asks for.
func parseOutline(given string, settings outlineSettings) (outlineSettings, error) {
	err := parseSettings(given, map[string]func(string) error{
		"style": func(value string) error {
			style, known := outlineStyle(value)
			if !known {
				return fmt.Errorf("'%s' is not an outline style; expected one of %s",
					value, outlineStyleList())
			}
			settings.Style = style
			return nil
		},
		"justify": settingFrom("an outline justification", outlineJustifications, &settings.Justify),
	})
	return settings, err
}

// listScopes are how much of the source the directory list reaches: the documents of the
// folder the document is in, those and the folders below it, or every document under the
// source, nested by folder.
var listScopes = []string{"current", "subfolders", "tree"}

// defaultList is what a directory list is drawn with before --list or the query names
// anything else.
var defaultList = listSettings{Style: "plain", Scope: "current"}

// parseList reads the "style" and "scope" settings onto the ones it is given, and returns the
// list the settings ask for.
func parseList(given string, settings listSettings) (listSettings, error) {
	err := parseSettings(given, map[string]func(string) error{
		"style": func(value string) error {
			style, known := outlineStyle(value)
			if !known {
				return fmt.Errorf("'%s' is not a list style; expected one of %s",
					value, outlineStyleList())
			}
			settings.Style = style
			return nil
		},
		"scope": settingFrom("a list scope", listScopes, &settings.Scope),
	})
	return settings, err
}

// contentPayload is the JSON the page polls for. Its mtime, text, css and base are null when
// the document could not be read.
//
// base is the route the document's relative links are resolved against, which the page cannot
// work out for itself: a route naming a folder resolves to a document inside it, and the
// browser would resolve the links beside the folder instead.
type contentPayload struct {
	MTime *string `json:"mtime"`
	Text  *string `json:"text"`
	CSS   *string `json:"css"`
	Base  *string `json:"base"`
	Error *string `json:"error"`
}

// version returns the version this binary was built from, or "dev" outside a module build.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	if err := run(); err != nil {
		logError("%sError: %v%s", colorRed, err, colorReset)
		os.Exit(1)
	}
}

// run parses the command line, resolves the source to serve, and serves it until interrupted.
func run() error {
	listenAddress := flag.String("listen-address", defaultListenAddress,
		"Address for the local web server to listen on")
	port := flag.Int("port", defaultServePort, "Port for the local web server")
	watchInterval := flag.Float64("watch-interval", 0, fmt.Sprintf(
		"Seconds between checks for changes to the file, polled by the browser page "+
			"(default %g, or %g for an %s URL, which is read over the network)",
		defaultWatchInterval, defaultADOWatchSecond, adoScheme))
	online := flag.Bool("online", false,
		"Load the Markdown and highlighting libraries from their CDNs instead of from inside this binary")
	outline := flag.String("outline", "", fmt.Sprintf(
		"Show an outline of the document's headings beside it, as a comma separated list of "+
			"settings: style:%s, justify:%s. A page takes an 'outline' query parameter of the "+
			"same settings, which overrides this one",
		outlineStyleList(), strings.Join(outlineJustifications, "|")))
	list := flag.String("list", "", fmt.Sprintf(
		"List the documents around the one on the page, on its left, as a comma separated list "+
			"of settings: style:%s, scope:%s. It takes the outline to the right, and a page "+
			"takes a 'list' query parameter of the same settings",
		outlineStyleList(), strings.Join(listScopes, "|")))
	mermaid := flag.Bool("mermaid", false, "Render fenced 'mermaid' blocks as diagrams")
	indexOnly := flag.Bool("index-only", false, fmt.Sprintf(
		"Read a folder as the first of %s it holds and look no further; a folder holding "+
			"neither is served as one holding no Markdown at all, rather than as a listing of "+
			"the files it does hold",
		strings.Join(localCandidates, " or ")))
	showVersion := flag.Bool("version", false, "Print the version and exit")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [flags] [path]\n\n"+
				"Render Markdown from local files or Azure Repos as GitHub-styled pages.\n\n"+
				"  path\n    \t%s\n\n"+
				"Flags stand before the path, which ends them; a flag written after it is not read:\n\n"+
				"  %s --list style:plain docs/\n    \tthe list is drawn\n"+
				"  %s docs/ --list style:plain\n    \tthe flag is ignored\n\nFlags:\n",
			filepath.Base(os.Args[0]), pathUsage,
			filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		logInfo("%s %s", filepath.Base(os.Args[0]), version())
		return nil
	}

	// Without an outline the style is empty and the side still stands, for a query to turn
	// the outline on without naming one.
	outlineOf := outlineSettings{Justify: defaultOutline.Justify}
	if *outline != "" {
		parsed, err := parseOutline(*outline, defaultOutline)
		if err != nil {
			return err
		}
		outlineOf = parsed
	}

	listOf := listSettings{Scope: defaultList.Scope}
	if *list != "" {
		parsed, err := parseList(*list, defaultList)
		if err != nil {
			return err
		}
		listOf = parsed
	}

	handler := &server{assets: embeddedAssets, online: *online, outline: outlineOf,
		list: listOf, mermaid: *mermaid, indexOnly: *indexOnly}
	if *online {
		handler.assets = cdnAssets
	}

	sourceDescription, err := handler.resolveStartupSource(flag.Arg(0))
	if err != nil {
		return err
	}

	handler.watchInterval = *watchInterval
	if handler.watchInterval <= 0 {
		handler.watchInterval = defaultWatchInterval
		if handler.defaultSource.kind == kindADO {
			handler.watchInterval = defaultADOWatchSecond
		}
	}

	address := net.JoinHostPort(*listenAddress, strconv.Itoa(*port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("could not start server on %s (%v); try a different --listen-address/--port", address, err)
	}

	printBanner("Serving Markdown")
	logInfo("  Source:   %s", sourceDescription)
	logInfo("  Watching: every %gs", handler.watchInterval)
	logInfo("  URL:      %shttp://%s%s%s", colorCyan, address, handler.sourceRoute(), colorReset)
	logInfo("")

	return handler.serve(listener)
}

// resolveStartupSource reads the positional path, sets the source the server serves by
// default, and returns the description of it printed in the banner.
func (s *server) resolveStartupSource(path string) (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	s.rootDir = workingDirectory

	switch {
	case strings.HasPrefix(path, adoScheme):
		src, err := parseADOURL(path)
		if err != nil {
			return "", err
		}
		s.defaultSource = src
		description := adoWebURL(src)
		if _, err := s.loadDocument(src); err != nil {
			return "", fmt.Errorf("cannot read %s (%v)", description, err)
		}
		return description, nil

	}

	s.defaultSource = source{kind: kindLocal}
	if path != "" {
		switch {
		case isFile(path):
			s.rootDir = filepath.Dir(path)
			s.defaultFile = path
		case isDir(path):
			s.rootDir = path
		default:
			return "", fmt.Errorf("'%s' does not exist or is not a file or directory", path)
		}
	}
	return fmt.Sprintf("'%s'", s.rootDir), nil
}

// serve runs the HTTP server on listener until the process is interrupted.
func (s *server) serve(listener net.Listener) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	httpServer := &http.Server{Handler: s}
	failed := make(chan error, 1)
	go func() {
		failed <- httpServer.Serve(listener)
	}()

	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logInfo("")
	logInfo("  %sStopping server%s", colorYellow, colorReset)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// ServeHTTP serves a page shell for every route, and the document that route addresses at
// /content. A route naming a picture or another binary file is answered with its bytes.
//
// A page route takes "outline" and "list" query parameters of the settings --outline and
// --list themselves take, which it applies onto them for that page; a parameter that does not
// read leaves them. A page holding a directory list carries its outline on the right.
func (s *server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, pageAssetRoute) {
		pageFiles.ServeHTTP(writer, request)
		return
	}

	if !s.online && strings.HasPrefix(request.URL.Path, assetRoute) {
		vendorFiles.ServeHTTP(writer, request)
		return
	}

	if request.URL.Path == "/content" {
		s.sendContent(writer, request.URL.Query().Get("path"))
		return
	}

	src, ok := resolveSource(request.URL.Path, s.defaultSource)
	if ok && isRawAsset(request.URL.Path) {
		s.sendRawAsset(writer, request, src)
		return
	}

	title := ""
	if ok {
		title = s.documentTitle(src)
	}
	if title == "" {
		body := fmt.Sprintf("no document found for '%s'\n\n%s\n", request.URL.Path, routeHelp)
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusNotFound)
		fmt.Fprint(writer, body)
		return
	}

	query := "?path=" + url.QueryEscape(request.URL.Path)
	page := renderPage(title, query, int(math.Round(s.watchInterval*1000)), s.assets,
		s.sidebarsFor(request, src), s.mermaid)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Write(page)
}

// sidebarsFor returns what stands beside the document src addresses: the server's own outline
// and list settings with the page's query applied onto them.
//
// A query naming any setting draws the sidebar it belongs to, taking the default style when
// the server was started without one, the way --outline and --list do. A query that does not
// read leaves the settings alone, and a list takes the outline to the right.
func (s *server) sidebarsFor(request *http.Request, src source) sidebars {
	beside := sidebars{Outline: s.outline, List: s.list}
	query := request.URL.Query()

	if given := query.Get("outline"); given != "" {
		base := beside.Outline
		if base.Style == "" {
			base.Style = defaultOutline.Style
		}
		if asked, err := parseOutline(given, base); err == nil {
			beside.Outline = asked
		}
	}
	if given := query.Get("list"); given != "" {
		base := beside.List
		if base.Style == "" {
			base.Style = defaultList.Style
		}
		if asked, err := parseList(given, base); err == nil {
			beside.List = asked
		}
	}

	if beside.List.Style != "" {
		beside.Entries, beside.Up = s.documentList(src, beside.List.Scope)
		carryQuery(beside.Entries, request.URL.RawQuery)
		beside.Up.Route = withQuery(beside.Up.Route, request.URL.RawQuery)
	}
	if beside.HoldsList() {
		beside.Outline.Justify = "right"
	}
	return beside
}

// sendContent answers a /content poll with the document the route addresses, or with the
// error that reading it raised.
func (s *server) sendContent(writer http.ResponseWriter, route string) {
	payload := contentPayload{}

	src, ok := resolveSource(route, s.defaultSource)
	if !ok {
		message := fmt.Sprintf("'%s' names no document", route)
		payload.Error = &message
	} else if read, err := s.loadDocument(src); err != nil {
		message := err.Error()
		payload.Error = &message
	} else {
		base := documentBase(route, read.name)
		payload.MTime, payload.Text, payload.CSS, payload.Base = &read.marker, &read.text, &read.css, &base
	}

	content, err := json.Marshal(payload)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write(content)
}
