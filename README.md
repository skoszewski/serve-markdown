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
| `--outline` | off | Show an outline of the document's headings beside it; takes `style` and `justify` as a comma separated list, e.g. `style:plain,justify:right` |
| `--list` | off | List a `dir:` or `ado://` source's documents on the left of the page; takes `style` and `scope`, e.g. `style:plain,scope:tree` |
| `--mermaid` | off | Render fenced `mermaid` blocks as diagrams |
| `--online` | off | Load the browser-side libraries from their CDNs rather than from inside the binary |
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
serve-markdown --outline style:nh --mermaid docs/
serve-markdown dir:docs
serve-markdown ado://myorg/myproject/myrepo/README.md
```

## Outline

`--outline` puts the document's structure beside it, built from its headings, each entry
linking to the heading it names. It takes its settings as a comma separated list of
`key:value` pairs:

```sh
serve-markdown --outline style:plain,justify:right docs/
```

| Setting | Values | Meaning |
|---|---|---|
| `style` | `plain` (`p`) | entries without numbers |
| | `numbered` (`n`) | entries numbered `1.`, `2.`, `3.` within each level |
| | `numbered-hierarchical` (`nh`) | entries numbered `1.`, `1.1.`, `1.1.1.` down the levels |
| | `none` | no outline |
| `justify` | `left`, `right` | the side of the document the outline stands on |

A setting left out keeps what it had, so `--outline justify:right` draws a `plain` outline on
the right, the styles' shorthands say the same as their names, and an unknown key or value
stops the server with a message naming it.

A page takes an `outline` query parameter of the same settings, applied onto the server's for
that page alone. A parameter that does not read leaves them:

```
http://127.0.0.1:8000/docs/guide.md?outline=style:nh,justify:right
http://127.0.0.1:8000/docs/guide.md?outline=style:none
```

The outline follows the document as it is re-read, and moves above it on a narrow window.

## Directory list

`--list` puts the documents around the one on the page on its left, for a `dir:` or an
`ado://` source; any other source is served without one. It takes its settings the way
`--outline` does:

```sh
serve-markdown --list style:plain,scope:subfolders dir:docs
```

| Setting | Values | Meaning |
|---|---|---|
| `style` | `plain` (`p`), `numbered` (`n`), `numbered-hierarchical` (`nh`) | how the entries are numbered |
| | `none` | no list |
| `scope` | `current` | the documents of the folder the page's document is in |
| | `subfolders` | those, the folders below it, and `..` to the folder above |
| | `tree` | every document under the source, nested by folder |

The document the page shows is marked, folders come before documents, and names beginning with
a dot are left out. Browsing keeps the query the page was opened with: every list entry, and
every link in the document itself that points back at the server, carries it on. A page carrying a list always carries its outline on the right, whatever
`justify` says. A page takes a `list` query parameter of the same settings:

```
http://127.0.0.1:8000/_/dir/guides/install.md?list=scope:tree
http://127.0.0.1:8000/_/dir/guides/install.md?list=style:none
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

A picture or a diagram linked from a document - `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`,
`.webp`, `.avif`, `.bmp`, `.ico` or `.pdf` - is served as the bytes it holds, so
`![](diagram.svg)` beside the document renders. Local files are read from the server's
directory, and an `ado://` document's pictures over the same REST API as the document itself.

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

With `--mermaid`, a fenced `mermaid` block is drawn as a diagram by
[mermaid](https://github.com/mermaid-js/mermaid), in the theme that preference asks for. The
bundle is loaded the first time a document holds such a block, so a document without diagrams
does not pay for it; a block mermaid cannot draw keeps its source visible.

They are served from copies built into the binary, so the page references no external host and
the server needs no network of its own. The copies and their licences are listed in
[assets/vendor/LICENSES.md](assets/vendor/LICENSES.md).

`--online` loads the same builds from their CDNs instead, leaving the embedded copies unused.
The page's own styling and script always come from the binary.

`./vendor.sh` on Linux and macOS, or `./vendor.ps1` wherever PowerShell Core runs, downloads
the libraries into `assets/vendor/` at the versions pinned at the top of the script; `list`
as the argument prints them with the URLs they come from instead.

## Licence

MIT, see [LICENSE](LICENSE).
