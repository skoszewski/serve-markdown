# serve-markdown

Serve Markdown from local files or Azure Repos as GitHub-styled HTML pages, from a single
executable with nothing else installed.

A page is served for every route; the browser polls the server for the document behind it and
re-renders as soon as it changes, so an editor and a browser side by side show the same file.

## Installation

```sh
go install github.com/skoszewski/serve-markdown@latest
```

From a clone, `./build.sh` on Linux and macOS, or `./build.ps1` wherever PowerShell Core runs,
writes the executable beside the sources. Both take the same action as their argument:

- `build`, the default, writes the executable;
- `test` runs the tests;
- `check` runs the formatting check, `go vet` and the tests;
- `clean` removes the executables.

A second argument names the destination platform as `<goos>/<goarch>`, building for it rather
than for the host and writing `serve-markdown-<goos>-<goarch>`. Go needs no extra toolchain to
do it, and the result is a static binary.

```sh
./build.sh build linux/arm64
```

```powershell
./build.ps1 build windows/amd64
```

## Usage

```
serve-markdown [flags] [path]
```

Flags must precede the path.

| Flag | Default | Meaning |
|---|---|---|
| `--listen-address` | `127.0.0.1` | Address for the local web server to listen on |
| `--port` | `8000` | Port for the local web server |
| `--watch-interval` | `1`, or `15` for `ado://` | Seconds between the browser page's checks for changes |
| `--offline` | off | Serve the browser-side libraries from inside the binary rather than from their CDNs |
| `--version` | | Print the version and exit |

The path names what to serve:

- a Markdown file, served at `/`;
- a directory, served at its own URL paths, resolving each to the `README.md` or `index.md`
  within it;
- `dir:<directory>`, which lists the Markdown files directly in a directory instead of
  requiring one of those names;
- `ado://<organization>/<project>/<repository>/<path to file>`, a file in an Azure Repos Git
  repository;
- nothing, which serves the current directory.

```sh
serve-markdown README.md
serve-markdown --port 9000 docs/
serve-markdown dir:docs
serve-markdown ado://myorg/myproject/myrepo/README.md
```

## Routes

Whatever the server was started with, these routes reach every kind of source while it runs:

```
/<path>                                              the source the server was started with
/_/file/<path>                                       a local file under the server's directory
/_/dir/<path>                                        a local directory's Markdown files, listed
/_/ado/<organization>/<project>/<repository>/<path>  a file in an Azure Repos repository
```

A path that resolves outside the directory the server was started in is served as not found.

## Documents

A file ending in `.md` or `.markdown` is rendered as Markdown. Any other file becomes a
document titled with its name, holding its contents in one fenced code block; the language
comes from the file's suffix, and the fence is made longer than the longest run of backticks
in the file, so a file that holds its own fences stays inside its block.

YAML front matter is stripped from the document rather than rendered. Its `css` key styles the
page:

```markdown
---
css: |
  .markdown-body { max-width: 1200px; }
---

# Notes
```

## Azure Repos

An `ado://` source reads the file over the Azure DevOps REST API, and takes its access token
from the Azure CLI, so `az login` must have been run. A path naming a folder, or ending in a
slash, resolves to the `README.md` or `index.md` within it. The change marker is the file's
Git object ID, so the page re-renders on every commit that touches it.

## Rendering

The page renders Markdown in the browser with [marked](https://github.com/markedjs/marked),
sanitises it with [DOMPurify](https://github.com/cure53/DOMPurify), highlights code with
[highlight.js](https://github.com/highlightjs/highlight.js), and styles it with
[github-markdown-css](https://github.com/sindresorhus/github-markdown-css), following the
browser's light or dark preference.

These are loaded from their CDNs. With `--offline` they are served from copies built into the
binary instead, and the page then references no external host. The copies and their licences
are listed in [assets/vendor/LICENSES.md](assets/vendor/LICENSES.md).

## Licence

MIT, see [LICENSE](LICENSE).
