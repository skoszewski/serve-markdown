package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// assetRoute is the URL prefix the embedded libraries are served under.
const assetRoute = "/" + routeNamespace + "/assets/"

// pageAssetRoute is the URL prefix the page shell's own styling and script are served under.
const pageAssetRoute = "/" + routeNamespace + "/page/"

//go:embed assets/vendor
var vendorFS embed.FS

//go:embed assets/page
var pageFS embed.FS

// vendorFiles serves the embedded libraries under assetRoute.
var vendorFiles = embeddedHandler(vendorFS, "assets/vendor", assetRoute)

// pageFiles serves the page shell's styling and script under pageAssetRoute, from the binary
// whether or not --online is given.
var pageFiles = embeddedHandler(pageFS, "assets/page", pageAssetRoute)

// embeddedHandler serves directory, as embedded in files, under route.
func embeddedHandler(files embed.FS, directory, route string) http.Handler {
	subtree, err := fs.Sub(files, directory)
	if err != nil {
		panic(err)
	}
	return http.StripPrefix(route, http.FileServer(http.FS(subtree)))
}
