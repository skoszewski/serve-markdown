package render

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// pageFS holds the page shell and its own styling, which are sources rather than built files.
//
//go:embed page
var pageFS embed.FS

// AssetHandler serves, under prefix, the built files in built at prefix + "assets/" and the
// page shell's styling at prefix + "page/"; any other route under prefix is not found.
//
// built holds what web/build.mjs writes: page.js, the Markdown and highlight stylesheets, and
// mermaid.min.js when diagrams are drawn. A nil built serves the styling alone.
func AssetHandler(prefix string, built fs.FS) http.Handler {
	styling, err := fs.Sub(pageFS, "page")
	if err != nil {
		panic(err)
	}
	pages := http.StripPrefix(prefix+"page/", http.FileServer(http.FS(styling)))
	var assets http.Handler = http.NotFoundHandler()
	if built != nil {
		assets = http.StripPrefix(prefix+"assets/", http.FileServer(http.FS(built)))
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, prefix+"page/"):
			pages.ServeHTTP(writer, request)
		case strings.HasPrefix(request.URL.Path, prefix+"assets/"):
			assets.ServeHTTP(writer, request)
		default:
			http.NotFound(writer, request)
		}
	})
}
