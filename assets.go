package main

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/skoszewski/serve-markdown/v3/render"
	"github.com/skoszewski/serve-markdown/v3/render/assets"
)

// assetPrefix is the URL prefix the page's assets are served under, as render.AssetHandler and
// render.PageSettings take it.
const assetPrefix = "/" + routeNamespace + "/"

// assetRoute is the URL prefix the built assets are served under.
const assetRoute = assetPrefix + "assets/"

// pageAssetRoute is the URL prefix the page shell's own styling is served under.
const pageAssetRoute = assetPrefix + "page/"

//go:embed assets/build
var buildFS embed.FS

//go:embed assets/page
var pageFS embed.FS

// editorBuild holds the editor's built files, editor.js and editor.css.
var editorBuild = func() fs.FS {
	files, err := fs.Sub(buildFS, "assets/build")
	if err != nil {
		panic(err)
	}
	return files
}()

// pageFiles serves the page's assets under assetPrefix.
var pageFiles = render.AssetHandler(assetPrefix, assets.FS)

// buildFiles serves the editor's built files under assetRoute.
var buildFiles = http.StripPrefix(assetRoute, http.FileServer(http.FS(editorBuild)))
