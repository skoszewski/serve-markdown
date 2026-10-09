package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// assetRoute is the URL prefix the built assets are served under.
const assetRoute = "/" + routeNamespace + "/assets/"

// pageAssetRoute is the URL prefix the page shell's own styling is served under.
const pageAssetRoute = "/" + routeNamespace + "/page/"

//go:embed assets/build
var buildFS embed.FS

//go:embed assets/page
var pageFS embed.FS

// buildFiles serves the assets web/build.mjs builds under assetRoute.
var buildFiles = embeddedHandler(buildFS, "assets/build", assetRoute)

// pageFiles serves the page shell's styling under pageAssetRoute.
var pageFiles = embeddedHandler(pageFS, "assets/page", pageAssetRoute)

// embeddedHandler serves directory, as embedded in files, under route.
func embeddedHandler(files embed.FS, directory, route string) http.Handler {
	subtree, err := fs.Sub(files, directory)
	if err != nil {
		panic(err)
	}
	return http.StripPrefix(route, http.FileServer(http.FS(subtree)))
}
