package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// assetRoute is the URL prefix the embedded libraries are served under.
const assetRoute = "/" + routeNamespace + "/assets/"

//go:embed assets/vendor
var vendorFS embed.FS

// vendorHandler serves the embedded libraries under assetRoute.
func vendorHandler() http.Handler {
	files, err := fs.Sub(vendorFS, "assets/vendor")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix(assetRoute, http.FileServer(http.FS(files)))
}
