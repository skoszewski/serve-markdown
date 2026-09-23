package main

import (
	"context"
	"encoding/json"
	"errors"
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

// contentPayload is the JSON the page polls for. Its mtime, text, css, base and info are null
// when the document could not be read.
//
// base is the route the document's relative links are resolved against, which the page cannot
// work out for itself: a route naming a folder resolves to a document inside it, and the
// browser would resolve the links beside the folder instead. info is what the front matter
// says of the document, its author and date left out under --front-matter title-only.
type contentPayload struct {
	MTime *string       `json:"mtime"`
	Text  *string       `json:"text"`
	CSS   *string       `json:"css"`
	Base  *string       `json:"base"`
	Info  *documentInfo `json:"info"`
	Error *string       `json:"error"`
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

// run reads what the server is to do, resolves the source to serve, and serves it until
// interrupted.
func run() error {
	settings, err := readConfiguration()
	if err != nil {
		return err
	}
	if settings.showVersion {
		logInfo("%s %s", filepath.Base(os.Args[0]), version())
		return nil
	}

	handler := &server{assets: embeddedAssets, online: settings.online, outline: settings.outline,
		list: settings.list, ado: settings.ado, contentWidth: settings.contentWidth, separators: settings.separators,
		frontMatter: settings.frontMatter, search: settings.search, mermaid: settings.mermaid,
		indexOnly: settings.indexOnly}
	if settings.online {
		handler.assets = cdnAssets
	}

	sourceDescription, err := handler.resolveStartupSource(settings.path)
	if err != nil {
		return err
	}
	// The seconds a local page checks at; a page reading Azure Repos checks for nothing.
	handler.watchInterval = settings.watchInterval(kindLocal)

	address := net.JoinHostPort(settings.listenAddress, strconv.Itoa(settings.port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("could not start server on %s (%v); try a different --listen-address/--port", address, err)
	}

	printBanner("Serving Markdown")
	logInfo("  Source:   %s", sourceDescription)
	if settings.file != "" {
		logInfo("  Config:   '%s'", settings.file)
	}
	if settings.watchInterval(handler.defaultSource.kind) > 0 {
		logInfo("  Watching: every %gs", handler.watchInterval)
	} else {
		logInfo("  Watching: on the browser's refresh")
	}
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
		src = s.adoVersion(src, "")
		s.defaultSource = src
		description := adoWebURL(src)
		if src.version != "" {
			description = fmt.Sprintf("%s (%s %s)", description, src.versionType, src.version)
		}
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
// A page route takes "outline", "list" and "ado" query parameters of the settings --outline,
// --list and --ado themselves take, which it applies onto them for that page; a parameter
// that does not read leaves them. A page holding a directory list carries its outline on the
// right.
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
		s.sendContent(writer, request)
		return
	}

	src, ok := resolveSource(request.URL.Path, s.defaultSource)
	src = s.adoVersion(src, request.URL.Query().Get("ado"))
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

	// The page reads its document at the version the page itself was asked for.
	query := "?path=" + url.QueryEscape(request.URL.Path)
	if given := request.URL.Query().Get("ado"); given != "" {
		query += "&ado=" + url.QueryEscape(given)
	}

	readVersions := s.versions
	if readVersions == nil {
		readVersions = versionsOf
	}

	watch := watchIntervalFor(src.kind, s.watchInterval)
	page := renderPage(title, query, int(math.Round(watch*1000)), s.assets, pageSettings{
		Sidebars: s.sidebarsFor(request, src), ContentWidth: s.contentWidth, Separators: s.separators,
		Mermaid: s.mermaid, Versions: readVersions(src)})
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
		if asked, err := parseList(given, beside.List); err == nil {
			beside.List = asked
		}
	}

	if beside.List.Scope != "" {
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
//
// The poll carries the route in its "path" parameter and, when the page was asked for one,
// the version to read Azure Repos at in its "ado" parameter.
func (s *server) sendContent(writer http.ResponseWriter, request *http.Request) {
	payload := contentPayload{}
	route := request.URL.Query().Get("path")

	src, ok := resolveSource(route, s.defaultSource)
	src = s.adoVersion(src, request.URL.Query().Get("ado"))
	if !ok {
		message := fmt.Sprintf("'%s' names no document", route)
		payload.Error = &message
	} else if read, err := s.loadDocument(src); err != nil {
		message := err.Error()
		payload.Error = &message
	} else {
		base := documentBase(route, read.name)
		if s.frontMatter == frontMatterTitleOnly {
			read.info.Author, read.info.Date = "", ""
		}
		payload.MTime, payload.Text, payload.CSS, payload.Base = &read.marker, &read.text, &read.css, &base
		payload.Info = &read.info
	}

	content, err := json.Marshal(payload)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write(content)
}
