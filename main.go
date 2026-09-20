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
	"A directory is served at its own URL path, resolving to index.md or README.md within it. " +
	"Prefix with 'dir:' and a directory to list its Markdown files instead of requiring one of " +
	"those, or give an 'ado://<organization>/<project>/<repository>/<path to file>' URL to serve " +
	"a file from an Azure Repos Git repository. Whichever is given, the /_/<scheme>/ routes " +
	"(file, dir, ado) reach the others while the server runs."

// contentPayload is the JSON the page polls for. Its mtime, text and css are null when the
// document could not be read.
type contentPayload struct {
	MTime *string `json:"mtime"`
	Text  *string `json:"text"`
	CSS   *string `json:"css"`
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
	showVersion := flag.Bool("version", false, "Print the version and exit")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [flags] [path]\n\n"+
				"Render Markdown from local files or Azure Repos as GitHub-styled pages.\n\n"+
				"  path\n    \t%s\n\nFlags (they must precede the path):\n",
			filepath.Base(os.Args[0]), pathUsage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		logInfo("%s %s", filepath.Base(os.Args[0]), version())
		return nil
	}

	handler := &server{assets: embeddedAssets, online: *online}
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
		if _, _, _, err := readADODocument(src); err != nil {
			return "", fmt.Errorf("cannot read %s (%v)", description, err)
		}
		return description, nil

	case strings.HasPrefix(path, dirPrefix):
		given := strings.TrimPrefix(path, dirPrefix)
		if !isDir(given) {
			return "", fmt.Errorf("'%s' does not exist or is not a directory", given)
		}
		s.rootDir = given
		s.defaultSource = source{kind: kindDir}
		return fmt.Sprintf("'%s' (Markdown file listing)", given), nil
	}

	s.defaultSource = source{kind: kindFile}
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
	if s.defaultFile == "" && findIndexFile(s.rootDir) == "" {
		return "", fmt.Errorf("no file given and none of %s found in '%s'",
			strings.Join(defaultCandidates, ", "), s.rootDir)
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
// /content.
func (s *server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !s.online && strings.HasPrefix(request.URL.Path, assetRoute) {
		vendorHandler().ServeHTTP(writer, request)
		return
	}

	if request.URL.Path == "/content" {
		s.sendContent(writer, request.URL.Query().Get("path"))
		return
	}

	src, ok := resolveSource(request.URL.Path, s.defaultSource)
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
	page := renderPage(title, query, int(math.Round(s.watchInterval*1000)), s.assets)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Write(page)
}

// sendContent answers a /content poll with the document the route addresses, or with the
// error that reading it raised.
func (s *server) sendContent(writer http.ResponseWriter, route string) {
	payload := contentPayload{}

	src, ok := resolveSource(route, s.defaultSource)
	if !ok {
		message := fmt.Sprintf("'%s' names no document", route)
		payload.Error = &message
	} else if marker, text, css, err := s.loadDocument(src); err != nil {
		message := err.Error()
		payload.Error = &message
	} else {
		payload.MTime, payload.Text, payload.CSS = &marker, &text, &css
	}

	content, err := json.Marshal(payload)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write(content)
}
