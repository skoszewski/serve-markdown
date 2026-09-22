# serve-markdown

Serve Markdown from local files or Azure Repos as GitHub-styled HTML pages, from a single
executable with nothing else installed.

A page is served for every route; for a local file the browser polls the server for the
document behind it and re-renders as soon as it changes, so an editor and a browser side by
side show the same file. A page reading Azure Repos is read once and again on refresh.

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

Flags stand before the path, which ends them; a flag written after it is not read:

```sh
serve-markdown --list style:plain docs/     # the list is drawn
serve-markdown docs/ --list style:plain     # the flag is ignored
```

| Flag | Default | Meaning |
|---|---|---|
| `--listen-address` | `127.0.0.1` | Address for the local web server to listen on |
| `--port` | `8000` | Port for the local web server |
| `--watch-interval` | `1` | Seconds between a local page's checks for changes; an `ado://` page makes none |
| `--outline` | off | Show an outline of the document's headings beside it; takes `style` and `justify` as a comma separated list, e.g. `style:plain,justify:right` |
| `--list` | off | List the documents around the page's own on its left; takes `style` and `scope`, e.g. `style:plain,scope:tree` |
| `--mermaid` | off | Render fenced `mermaid` blocks as diagrams |
| `--search` | `README.md,index.md` | The documents a folder is read as, looked through in the order written |
| `--index-only` | off | Read a folder as its index document alone, looking no further |
| `--content-width` | `full` | How wide the document is drawn: `small`, `medium`, `large` or `full` |
| `--ado` | the default branch | Read Azure Repos at a version: `branch:release/2.1`, `tag:v1.0` or `commit:9a3f2b1` |
| `--config` | `serve-markdown.yaml` beside the path | Read the flags from a YAML file |
| `--online` | off | Load the browser-side libraries from their CDNs rather than from inside the binary |
| `--version` | | Print the version and exit |

The path names what to serve:

- a Markdown file, served at `/`;
- a directory, served at its own URL paths;
- `ado://<organization>/<project>/<repository>/<path to file>`, a file in an Azure Repos Git
  repository, served under `/_/ado/<organization>/<project>/<repository>`;
- nothing, which serves the current directory.

A directory - the one the server was started with or any below it - is the first document of
`--search` it holds, `README.md` then `index.md` unless another list is given; a directory
holding none of them is served as a list of its Markdown files, and one holding no Markdown
file at all as a page saying so. An `ado://` folder is read the same way, the search being one
list for both:

```sh
serve-markdown --search index.md,README.md docs/
serve-markdown --search HOME.md ado://myorg/myproject/myrepo
```

`--index-only` stops the looking there: a folder is its index document or nothing, and a
folder holding none is served as the page saying no Markdown files were found, whatever else
stands in it.

Within an `ado://` repository the list beside the page follows that rule too, holding the
folders and their index documents alone, since a document the page will never open is not
somewhere to browse. A folder contributes one document at most - the one it is read as, the
first of `--search` standing there - or none. A repository root left with nothing to browse
then falls back to the project's repositories, the way it does for a repository holding its
index document alone. A local directory's list still names every Markdown file it finds.

An `ado://` source is reached through its own route alone, since the repository is named by
the route itself, and the server prints that route at startup.

```sh
serve-markdown README.md
serve-markdown --port 9000 docs/
serve-markdown --outline style:nh --mermaid docs/
serve-markdown ado://myorg/myproject/myrepo/README.md
```

## Configuration file

The flags may be written in a YAML file instead, one key per flag, named as the flag is. A
`serve-markdown.yaml` beside the path is read when there is one - the directory the path
names, the directory of the file it names, or the directory the server was started in for an
`ado://` source - and `--config` names another file, which must be there.

```yaml
index-only: true
list: true
outline:
  style: numbered-hierarchical
  justify: right
content-width: large
port: 9000
```

A flag taking settings - `outline`, `list`, `ado` - is written as a mapping of those settings,
as `true` to draw it with its own, or as `false` to leave it out. Every other flag takes the
value its kind asks for: a string, a number, or `true` and `false`.

A flag written on the command line stands above what the file says, so a file can serve a
directory while one run tweaks a setting:

```sh
serve-markdown --outline style:plain docs/     # the file's outline settings are set aside
serve-markdown --config ~/reading.yaml docs/   # another file, read instead of the one beside
```

A key no flag is named after, a setting a flag does not know, or a value of the wrong kind
stops the server with a message naming it and the line it stands on. The file that was read is
printed at startup.

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

### A query stands before the fragment

An outline entry, and many a link within a document, addresses a heading by a fragment -
`guide.md#steps`. A query parameter goes *before* that fragment, never after it:

```
http://127.0.0.1:8000/docs/guide.md?outline=style:none#steps     the outline is left out
http://127.0.0.1:8000/docs/guide.md#steps?outline=style:none     nothing happens
```

A URL reads `path?query#fragment`, and everything after the `#` is the fragment, `?` and `&`
included. The browser keeps the fragment to itself and never sends it, so a parameter written
there reaches no server; the heading is not found either, the page looking for one named
`steps?outline=style:none`. Every browser behaves this way - it is what a URL means - and the
page is served as though the parameter had not been written at all.

## Directory list

`--list` puts the documents around the one on the page on its left. It takes its settings the
way `--outline` does:

```sh
serve-markdown --list style:plain,scope:subfolders docs/
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
every link in the document itself that points back at the server, carries it on. A page
carrying a list always carries its outline on the right, whatever `justify` says. A page takes
a `list` query parameter of the same settings:

```
http://127.0.0.1:8000/guides/install.md?list=scope:tree
http://127.0.0.1:8000/guides/install.md?list=style:none
```

It stands before the fragment, as every query parameter does.

## Routes

Whatever the server was started with, these routes reach both kinds of source while it runs:

```
/<path>                                              a local file or directory under the server's directory
/_/ado/<organization>                                the projects of an Azure DevOps organization
/_/ado/<organization>/<project>                      the Git repositories of a project
/_/ado/<organization>/<project>/<repository><path>   a file in an Azure Repos repository
```

An organization and a project hold no document of their own, so each is served as a list of
what it holds, every entry opening the route below it; a repository is read as its documents.
The first five segments of a repository route are the address of the repository itself, and
the path within it follows them, so `/_/ado/org/project/repo`, with or without a trailing
slash, is the repository's own document.

A route may name a folder as well as a document, with or without a trailing slash; the folder
resolves to the document within it, and the routes themselves are served as written. The
relative links the document holds - `pool/README.md`, `../README.md`, an image beside it - are
resolved against the folder the document was read from, which the server sends to the page
with the document.

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
slash, resolves to the first document of `--search` within it.

An `ado://` page does not check for changes on its own: a local file changes as it is written,
while a repository changes when someone pushes to it, so the page is read once and read again
when the browser is refreshed. The document, the sidebar and the pictures are all read afresh
then. A local page keeps checking every `--watch-interval` seconds.

What an organization, a project and a repository hold - its projects, its Git repositories,
its branches and tags - is kept for half an hour after it is read, the same half hour the
access token is kept for. Those listings
back the sidebar of every page below them, and they change when someone creates or deletes a
project or a repository rather than while a document is being read. A project or repository
made meanwhile appears once the half hour is up, or when the server is started again; a
repository's own documents are read afresh every time.

A folder holding neither is served as a page titled after it - after the repository at its
root - naming the documents of `--search` it does not hold; the documents it does hold
are named by the list beside the page. With `--index-only` the page says that no Markdown
files were found, nothing else having been looked for.

A URL may stop short of a file:

```sh
serve-markdown ado://myorg                            # the organization's projects
serve-markdown ado://myorg/myproject                  # the project's repositories
serve-markdown ado://myorg/myproject/myrepo           # the repository's own document
serve-markdown ado://myorg/myproject/myrepo/docs/     # a folder within it
```

An organization and a project are folders holding no document of their own, so each is served
as a list of what it holds - its projects, its Git repositories - each entry opening the one
below it. A disabled repository is left out.

### Version

A repository is read at its default branch unless `--ado` names another version, as a comma
separated list of settings:

| Setting | Meaning |
|---|---|
| `branch:<name>` | a branch, `branch:release/2.1` |
| `tag:<name>` | a tag, `tag:v1.0` |
| `commit:<id>` | a commit, `commit:9a3f2b1` |

One list names one version: naming two stops the server with a message saying which had it
already.

```sh
serve-markdown --ado branch:release/2.1 ado://myorg/myproject/myrepo
serve-markdown --ado tag:v1.0 ado://myorg/myproject/myrepo/docs/guide.md
```

A page takes an `ado` query parameter of the same settings, replacing the version for that
page and carried on while browsing, so a whole repository can be read at a tag without
restarting the server:

```
http://127.0.0.1:8000/_/ado/myorg/myproject/myrepo?ado=tag:v1.0
http://127.0.0.1:8000/_/ado/myorg/myproject/myrepo/docs/guide.md?ado=commit:9a3f2b1
```

It stands before the fragment, as every query parameter does.

The version reaches everything read from the repository: the document, the pictures beside it
and the file list. A project's repositories and an organization's projects have no version of
their own, so it does not touch them.

A page reading a repository draws a picker above the document: a box naming the branch the
repository is read at unasked, a **Branch** and a **Tag** button, one of them chosen at a
time, and a list of what that button holds. A repository holding no tags is offered its
branches alone, the two buttons being left out where there is nothing to choose between.
Choosing a name opens the same document at that version, which is the `ado` parameter written
for you. Both lists come with the page, so
moving between branches and tags asks the server for nothing; each is read from Azure DevOps
once and kept for half an hour, as the projects and repositories are. A repository whose
branches cannot be read carries no picker.

With `--list` - written before the URL, the way every flag is - the sidebar follows the level
above the page, so the way back is always beside the way down:

| Page | Sidebar |
|---|---|
| an organization | the organization's projects |
| a project | the same, the one on the page marked |
| a repository's root | the repository's documents, under a **Browse to repositories** link back to the project |
| a folder within it | the folder's documents, under an **Up** link to the folder above |

The `scope` says how far the sidebar reaches below its entries, above a repository as within
one:

| Scope | An organization or a project shows |
|---|---|
| `current` | the projects alone |
| `subfolders` | the projects, with the repositories of the one on the page below it |
| `tree` | the projects, each with its repositories - one read of the repositories per project |

A repository whose root would list nothing but the document already on the page - a repository
with its index document and no other Markdown - carries the project's repositories instead,
the one on the page marked, and the link above them reads **Back to projects**, leading to the project
the way it always does. A repository holding one document under another name keeps it, since
that entry is the only way to reach it, and a folder within a repository always keeps its own
list, however short, the **Up** link being the way out of it.

## Rendering

The page renders Markdown in the browser with [marked](https://github.com/markedjs/marked),
sanitises it with [DOMPurify](https://github.com/cure53/DOMPurify), highlights code with
[highlight.js](https://github.com/highlightjs/highlight.js), and styles it with
[github-markdown-css](https://github.com/sindresorhus/github-markdown-css), following the
browser's light or dark preference.

`--content-width` says how wide the document is drawn. Each sidebar takes 320px and the
document's own padding 45px a side, so the text is the width below less 90px, and less again
what the sidebars take:

| Width | The document is capped at | Text at that cap |
|---|---|---|
| `small` | 780px | 690px, about 86 characters |
| `medium` | 980px | 890px, what `github-markdown-css` is written for |
| `large` | 1280px | 1190px, a 1080p window less both sidebars |
| `full` | the window | whatever the sidebars leave |

A cap wider than the window is simply not reached: on a 1512px screen with both sidebars every
width above `small` draws the same 872px column.

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
