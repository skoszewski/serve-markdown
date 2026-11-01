// Package assets holds the browser files web/build.mjs builds into render/assets/build, for a
// program built from a clone of serve-markdown after the build: the files are not kept in the
// repository, so the package does not compile without them.
//
// A program importing render from the module supplies these files itself, as the
// serve-markdown-assets archive of a release unpacks them.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed build
var build embed.FS

// FS holds page.js, the Markdown and highlight stylesheets and mermaid.min.js, as
// render.AssetHandler and render.Options take them.
var FS = func() fs.FS {
	files, err := fs.Sub(build, "build")
	if err != nil {
		panic(err)
	}
	return files
}()
