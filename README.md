# serve-markdown

Serve Markdown from local files or Azure Repos as GitHub-styled HTML pages, from a single
executable with nothing else installed.

A page is served for every route; for a local file the browser polls the server for the
document behind it and re-renders as soon as it changes, so an editor and a browser side by
side show the same file. A page reading Azure Repos is read once and again on refresh.

## Installation

Every release carries the executable for Linux, macOS and Windows on `amd64` and `arm64`, to
download from the repository's Releases page as
`serve-markdown-<version>-<goos>-<goarch>.tar.gz`, or `.zip` on Windows, where `<version>` is
the release's version without the `v`, holding `serve-markdown` (`serve-markdown.exe` on
Windows):

```sh
tar -xzf serve-markdown-3.1.3-darwin-arm64.tar.gz
./serve-markdown --version
```

The `build` workflow in `.github/workflows/build.yml` builds the browser assets,
checks the formatting, vets and tests the sources on every push to `dev`, builds the
executables for `darwin/arm64` and `windows/amd64`, and publishes them as the `dev-latest`
prerelease, replacing the previous one, so they download from
`https://github.com/skoszewski/serve-markdown/releases/download/dev-latest/serve-markdown-X.Y.Z-dev-YYYYMMDD-<goos>-<goarch>.tar.gz`
(`.zip` for Windows).
Such a build names itself `vX.Y.Z-dev-YYYYMMDD`: the latest release before it and its UTC
build date; its files carry that version without the `v`. Pushing a semver tag with a leading `v` - `v1.2.0`, `v1.3.0-rc.1` - on a commit on
`main` checks the sources, builds all six and publishes them as the release of that tag;
`main` is pushed before the tag or together with it, as `./build.mjs release` does.

A release and `dev-latest` also carry `ServeMarkdown-<version>-windows-amd64.zip`: the Windows
notification-area app `ServeMarkdown.exe` with `serve-markdown.exe` beside it, for Windows x64.
The app is self-contained, carrying its own .NET runtime, so it is unzipped and run with nothing
else installed. It is built on a Windows runner. They also carry
`ServeMarkdown-<version>-darwin-arm64.zip`:
the macOS menu bar app `ServeMarkdown.app` with `serve-markdown` inside it, for Apple silicon,
built on a macOS runner (see [macOS menu bar app](#macos-menu-bar-app)).

From a clone, with Go and Node.js installed, `./build.mjs` writes the executable beside the
sources; on Windows it is run as `node build.mjs`. It takes the action as its argument:

- `build`, the default, writes the executable;
- `test` runs the tests;
- `check` runs the formatting check, `go vet` and the tests;
- `clean` removes the executables, the menu bar app, the Windows app and their builds;
- `macos`, on macOS alone, builds the menu bar app `dist/ServeMarkdown.app` (see
  [macOS menu bar app](#macos-menu-bar-app));
- `windows` builds the Windows notification-area app and `serve-markdown.exe` into
  `dist/ServeMarkdown-windows`, for Windows x64, with the .NET 10 SDK installed. The app runs on
  the installed .NET Desktop Runtime, and `windows self-contained` makes it carry its own. Built
  anywhere but Windows, the app opens a console window beside its icon and has no icon of its
  own, as .NET builds a Windows app on another platform;
- `assets` builds the browser assets alone (see [Browser assets](#browser-assets));
- `update` moves every package `web/package.json` names to its latest release, major versions
  included, rewriting `web/package-lock.json`, and builds the assets from them. A new esbuild
  release runs its install script only once `npm install-scripts approve esbuild` in `web/`
  has approved it;
- `release vX.Y.Z` runs `check` on a clean `dev`, opens `$EDITOR` for the release notes,
  prefilled with the commit subjects since the previous tag under `## Changed`, makes the
  annotated tag `vX.Y.Z` on `dev` with them, fast-forwards `main` to `dev`, and pushes `dev`,
  `main` and the tag in one atomic push. Lines starting with `;` are dropped from the notes.

Every action but `clean` first installs the packages pinned in `web/package-lock.json` and
builds the browser assets into `assets/build/`, since the executable embeds them. A plain
`go build` or `go test` works once they have been built.

A second argument names the destination platform as `<goos>/<goarch>`, building for it rather
than for the host and writing `serve-markdown-<goos>-<goarch>`. Go needs no extra toolchain to
do it, and the result is a static binary.

```sh
./build.mjs build linux/arm64
```

## Usage

```
serve-markdown [flags] [path]
```

Flags stand before the path, which ends them; a flag written after it is not read:

```sh
serve-markdown --list scope:current docs/     # the list is drawn
serve-markdown docs/ --list scope:current     # the flag is ignored
```

| Flag | Default | Meaning |
|---|---|---|
| `--listen-address` | `127.0.0.1` | Address for the local web server to listen on |
| `--port` | `8000` | Port for the local web server |
| `--watch-interval` | `1` | Seconds between a local page's checks for changes; an `ado://` page makes none |
| `--outline` | off | Show an outline of the document's headings beside it; takes `style`, `justify` and `display` as a comma separated list, e.g. `style:plain,justify:right` |
| `--list` | off | List the documents around the page's own on its left; takes `scope` and `display` as a comma separated list, e.g. `scope:tree,display:hidden` |
| `--mermaid` | off | Render fenced `mermaid` blocks as diagrams |
| `--search` | `README.md,index.md` | The documents a folder is read as, looked through in the order written |
| `--index-only` | off | Read a folder as its index document alone, looking no further |
| `--css` | none | CSS styling every document whose front matter declares none, see [Documents](#documents) |
| `--content-width` | `full` | How wide the document is drawn: `small`, `medium`, `large` or `full` |
| `--front-matter` | `all` | What the top row shows of the document's front matter: `all` or `title-only` |
| `--separators` | `side` | The lines separating the top row, the sidebars and the document: `hidden`, `top`, `side` or `all` |
| `--ado` | the default branch | Read Azure Repos at a version: `branch:release/2.1`, `tag:v1.0` or `commit:9a3f2b1` |
| `--editor` | off | Edit the documents of the served directory at `/_/editor/`, see [Editor](#editor) |
| `--editor-templates` | none | The directory of the templates new documents are created from |
| `--editor-config` | `editor.json` in the served directory | The editor configuration file to read, which must exist and read, see [Editor](#editor) |
| `--editor-exclude` | none | Comma separated list of the path patterns left out of editing |
| `--editor-spell-check` | off | Show the spell check switch and language in the editor's top bar |
| `--config` | see [Configuration file](#configuration-file) | Read the server configuration from the file |
| `--pdf` | | Print the Markdown document the path names to this PDF file and exit, see [PDF export](#pdf-export) |
| `--pdf-page` | see [PDF export](#pdf-export) | The page `--pdf` prints on, as a comma separated list of settings, e.g. `paper:letter,font-size:11pt` |
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

The server configuration may be written in a YAML file instead, one key per flag, named as the
flag is. The first file found of these is read, and the rest are not:

1. the file `--config` names, which must be there;
2. `serve-markdown.yaml` in the directory the server was started in;
3. `serve-markdown/config.yaml` in the user's configuration directory: `$XDG_CONFIG_HOME`, or
   `$HOME/.config` without it, on Linux; `$HOME/.config`, then
   `$HOME/Library/Application Support` on macOS; `%AppData%` on Windows.

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
serve-markdown --config ~/reading.yaml docs/   # another file, read instead of the others
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
| `display` | `shown`, `hidden` | whether the outline is shown or hidden until the reader chooses with its button |

A setting left out keeps what it had, so `--outline justify:right` draws a `plain`, `shown`
outline on the right, the styles' shorthands say the same as their names, and an unknown key or value
stops the server with a message naming it.

A page takes an `outline` query parameter of the same settings, applied onto the server's for
that page alone. A parameter that does not read leaves them:

```
http://127.0.0.1:8000/docs/guide.md?outline=style:nh,justify:right
http://127.0.0.1:8000/docs/guide.md?outline=style:none
```

The outline follows the document as it is re-read, and moves above it on a narrow window.

Every page carries a top row above the sidebars and the document, holding the control buttons
on its right, after the server's name and version, `Serve Markdown v2.3.0`: the list button first, on a page with a directory list (see
[Directory list](#directory-list)), then the outline button, then the theme button (see
[Rendering](#rendering)). A page with an outline carries
the outline button there, drawn as a window with
the outline's panel on the side the outline stands on: filled while the outline is shown, and
empty once pressing it has hidden the outline and left the document the room it took. Pressing
it again brings the outline back.
The browser remembers the choice for every page of the same server, across pages and restarts,
until the button is pressed again; `display` says how the outline starts before the reader has
chosen.

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
way `--outline` does, `scope` and `display` being the ones it knows:

```sh
serve-markdown --list scope:subfolders docs/
```

| Setting | Values | Meaning |
|---|---|---|
| `scope` | `current` | the documents of the folder the page's document is in |
| | `subfolders` | those, the folders below it, and `..` to the folder above |
| | `tree` | every document under the source, nested by folder |
| | `none` | no list |
| `display` | `shown`, `hidden` | whether the list is shown or hidden until the reader chooses with its button |

The entries are names to open and carry no numbering of their own; `style` belongs to
`--outline` alone.

A page with a list carries the list button in the top row, drawn as a window with the list's
panel on the left: filled while the list is shown, and empty once pressing it has hidden the
list. The browser remembers the choice for every page of the same server, apart from the
outline's, until the button is pressed again; `display` says how the list starts before the
reader has chosen.

The document the page shows is marked, folders come before documents, and names beginning with
a dot are left out.

A folder below the served directory holding nothing but the document on the page - or nothing
at all - is listed as the folder above it, with the folder itself marked and `..` leading
further up, in the `current` and `subfolders` scopes alike. The served directory keeps its own
list, however short, and `tree` lists the whole tree as it is. Browsing keeps the query the page was opened with: every list entry, and
every link in the document itself that points back at the server, carries it on. A page
carrying a list always carries its outline on the right, whatever `justify` says. A page takes
a `list` query parameter of the same settings:

```
http://127.0.0.1:8000/guides/install.md?list=scope:tree
http://127.0.0.1:8000/guides/install.md?list=scope:none
```

It stands before the fragment, as every query parameter does.

## Routes

Whatever the server was started with, these routes reach both kinds of source while it runs:

```
/<path>                                              a local file or directory under the server's directory
/_/ado/<organization>                                the projects of an Azure DevOps organization
/_/ado/<organization>/<project>                      the Git repositories of a project
/_/ado/<organization>/<project>/<repository><path>   a file in an Azure Repos repository
/_/editor/                                           the editor, with --editor, see Editor
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
page, and its `title`, `author` and `date` keys describe the document:

```markdown
---
title: Release notes
author: Ann Smith
date: 2024-05-01
css: |
  .markdown-body { max-width: 1200px; }
---

# Notes
```

The title is centred in the top row and names the browser's tab, which is otherwise named after
the file. The author and the date stand under it a third smaller,
joined by ` - ` when both are given; without a title neither is shown. `--front-matter
title-only` shows the title alone. A date written without a time is shown as the date alone,
one with a time as `2024-05-01 10:30`, and one YAML does not read as a date as it is written. A
key holding a list or a mapping is not shown.

`--css`, written in the configuration file as `css`, styles every document whose front matter
declares no `css` of its own, a document without front matter among them. A document's own
`css` replaces it rather than adding to it, so a document keeps the styling it was written
with:

```yaml
css: |
  .markdown-body table {
    display: table;
    width: 100%;
  }
```

A selector written for a document is to start with `.markdown-body`, as the stylesheet's own
do; a bare `table` rule loses to the stylesheet's `.markdown-body table`.

A picture or a diagram linked from a document - `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`,
`.webp`, `.avif`, `.bmp`, `.ico` or `.pdf` - is served as the bytes it holds, so
`![](diagram.svg)` beside the document renders. Local files are read from the server's
directory, and an `ado://` document's pictures over the same REST API as the document itself.

## Azure Repos

An `ado://` source reads the file over the Azure DevOps REST API. A path naming a folder, or
ending in a slash, resolves to the first document of `--search` within it.

The request is authenticated with the personal access token `AZURE_DEVOPS_PAT` holds, and
with an access token from the Azure CLI when that variable is empty, which asks that
`az login` has been run:

```sh
AZURE_DEVOPS_PAT=$(cat ~/.ado-token) serve-markdown ado://myorg/myproject/myrepo
```

A token needs the **Code (read)** scope, and is carried as basic authentication under an empty
user name, the way Azure DevOps takes it. Reading a repository without the Azure CLI installed
- from a container among others - asks for that variable.

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

The theme button on every page goes from the browser's preference, drawn as a half-filled
circle, to light, drawn as a sun, to dark, drawn as a moon, and back. The page, the code and
the diagrams all take the theme chosen, and the browser remembers it for every page of the
same server until the button is pressed again.

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

`--separators` says which lines separate the page's boxes. It is set on the command line or in
the configuration file; a page takes no query parameter for it.

| Value | Lines drawn |
|---|---|
| `all` | the line under the top row and the lines along the sidebars |
| `top` | the line under the top row alone |
| `side` | the lines along the sidebars alone, and between them on a narrow window |
| `hidden` | none |

A line left out keeps its room, so nothing moves between the values.

With `--mermaid`, a fenced `mermaid` block is drawn as a diagram by
[mermaid](https://github.com/mermaid-js/mermaid), in the theme that preference asks for. The
bundle is loaded the first time a document holds such a block, so a document without diagrams
does not pay for it; a block mermaid cannot draw keeps its source visible.

### Browser assets

Everything the page loads is served from inside the binary, so the page references no external
host and the server needs no network of its own.

The assets are built from `web/`: `web/package.json` names the libraries, `web/package-lock.json`
pins their versions, and `web/build.mjs` builds them with [esbuild](https://esbuild.github.io)
into `assets/build/`:

| File | Built from |
|---|---|
| `page.js` | `web/src/page.js` bundled with marked, DOMPurify and highlight.js |
| `github-markdown-light.css`, `github-markdown-dark.css` | github-markdown-css's light and dark themes |
| `highlight-light.css`, `highlight-dark.css` | highlight.js's GitHub themes |
| `mermaid.min.js` | mermaid's single-file bundle, copied as it is |
| `editor.js`, `editor.css` | `web/src/editor/` bundled with React, MDXEditor and Headless UI, type checked first with `web/tsconfig.json` |
| `LICENSES.md` | the name, version and licence of every package above |

`web/src/highlight.js` chooses the languages code is highlighted in: highlight.js's common set
and PowerShell (`powershell`, `pwsh`, `ps`, `ps1`). Another language is one import and one
`registerLanguage` line there.

`assets/build/` is not kept in the repository: `./build.mjs` builds it before the executable,
the `build` workflow before every check and build, and the `Dockerfile` in a Node.js stage of
its own.

## Editor

`--editor` serves a visual Markdown editor of the served directory at `/_/editor/`, printed at
startup beside the URL. It edits a local directory, so an `ado://` source with `--editor` stops
the server.

```sh
serve-markdown --editor --editor-templates ../templates --editor-exclude reference,*.draft.md docs/
```

The editor lists the documents (`.md`, `.markdown`) below the directory, names beginning with a
dot left out, and opens the one chosen in [MDXEditor](https://mdxeditor.dev), with a toolbar for
the headings, the formatting, the lists, links, pictures, tables and code blocks, and a switch to
the Markdown source and to the differences from the saved file.

`Ctrl+S` or `Cmd+S` saves the file in place. The **File** menu creates a document, a document
from a template, or a folder with an `index.md`, and saves or reverts the open file;
leaving a file with unsaved changes asks to save or revert them. The top bar holds the document
properties, the preview, which opens the page the server renders of the saved file, the
settings, and the theme button, which goes from the browser's preference, drawn as a
half-filled circle, to light, drawn as a sun, to dark, drawn as a moon, and back, the way the
pages' own does and sharing its choice with them. The panel on the right is the outline of the document's headings; both side panels
are resized by dragging their edges and hidden with the buttons at the ends of the top bar.

The properties are kept in the front matter: `title`, `author` and `tags` are edited, `created`
is set when the editor creates the document and `updated` whenever it saves it. The other keys
and the comments of the front matter are kept as written. A new document is titled after its
name, or after its folder for an `index.md` or `README.md`, dashes and underscores read as
spaces.

A template is a Markdown file of the `--editor-templates` directory; its `title`, `author` and
`description` describe it in the list, and a document created from it takes its body, with the
author from `git config user.name` and `user.email`.

An `editor.json` in the served directory, read at startup and printed in the banner, names the
folders below it whose documents are edited and the paths left out, the way a repository keeps
the settings of its own documents:

```json
{
  "directories": ["docs", "guides"],
  "exclude": ["tmp", "node_modules", "docs/reference"]
}
```

Both keys may be left out: without `directories` the whole served directory is edited, and a
new document must be created within one of them. Its `exclude` patterns are added to those of
`--editor-exclude`. A missing file, or one that does not read - an unknown key, a folder outside
the served directory, or a pattern that does not read - is warned of in the banner and set aside,
the editor then editing the whole served directory but for what `--editor-exclude` names.

`--editor-config` names the file to read in place of `editor.json`, wherever it stands; its
folders are still relative to the served directory. The file named must exist and read, or the
server stops with a message naming it, and the flag needs `--editor`:

```sh
serve-markdown --editor --editor-config ~/docs-editor.json docs/
```

Exclusion patterns are matched as `path.Match` globs: one without a slash against the name of the
file and of every folder it lies in, one with a slash against its path below the served
directory and the path of every folder it lies in. `reference` leaves out every folder named so,
`docs/reference` that one folder. `**` and the `!` of `.gitignore` are not read.

The settings, kept in the browser, choose:

| Setting | Values |
|---|---|
| Colour theme | GitHub, the default, Visual Studio Code (Light+ and Dark+) - white and blue, Solarized, Catppuccin (Latte and Mocha), Gruvbox, Monochrome (Radix Gray) - greys alone |
| Appearance | the browser's preference, light or dark, shared with the theme button of the pages |
| Document format | Markdown, the default, or MDX (Markdown with JSX), see below |
| Text button labels | text rather than icons on the top bar buttons |
| Save and Revert buttons | the two buttons in the top bar as well as in the **File** menu |
| Git controls | the branch, **Fetch**, **Pull**, **Commit** and **Push** above the file list |

The document format says how a document is read. **Markdown** reads it as CommonMark with
GitHub's extensions, as the pages render it: HTML - a block of its own, or a tag within a
paragraph or a table cell - is kept as written, shown as its source, and edited in source mode,
while the Markdown around it stays editable. An autolink such as `<https://example.com>` is
written back as `[https://example.com](https://example.com)`, which renders the same. **MDX**
reads every tag as JSX, which takes `<br/>` but not `<br>`, nor an autolink; a document holding
them is refused with a parse error, and opens in source mode alone.

The Git controls run `git` in the served directory: **Commit** adds and commits the changed
editable files with the message given, **Push** sets the upstream branch on the first remote of
a branch that has none, **Pull** fast-forwards only, and switching the branch reloads the file
list and the open file.

The editor writes files, so it is off unless asked for, and it answers a request changing a file
only from its own page: a request from another origin, or with a body that is not JSON, is
refused.

## PDF export

`--pdf` prints one Markdown document to a PDF file instead of serving it, and exits. The path
is then required and names the document: a local Markdown file, or an `ado://` URL of a
Markdown file. A directory, or an `ado://` organization, project, repository or folder, is
refused.

```sh
serve-markdown --pdf guide.pdf docs/guide.md
serve-markdown --mermaid --pdf-page paper:letter,orientation:landscape --pdf guide.pdf docs/guide.md
serve-markdown --pdf guide.pdf ado://myorg/myproject/myrepo/docs/guide.md
```

The document is drawn by the same page as in a browser, by headless Chrome, so the PDF reads
as the page does, in the light theme. It holds the title row the front matter gives, and the
document; the buttons, the sidebars and the version picker are left out. Long code lines wrap,
the headings become the PDF's bookmarks, and `--mermaid` draws the diagrams. Links to other
hosts and to headings within the document are kept; a link to another document of the
server is printed as its text, since the server is gone once the PDF is written.

`--pdf-page` takes the page the PDF is printed on, written on the command line, as `pdf-page`
in the configuration file, or as `pdf` in the document's own front matter:

| Setting | Values | Default |
|---|---|---|
| `paper` | `a3`, `a4`, `a5`, `letter`, `legal`, `tabloid` | `a4` |
| `orientation` | `portrait`, `landscape` | `portrait` |
| `margin` | 1 to 4 lengths in `mm`, `cm`, `in` or `pt`, as CSS writes a margin | `20mm` |
| `header` | `none`, `title` - the document's title | `none` |
| `footer` | `none`, `page` - the page number, `pages` - the page number of the number of pages | `pages` |
| `font-size` | the size of the document's text in `pt` or `px`, e.g. `11pt` | `16px`, the stylesheet's own |

The headings, the code, the tables and the rest of the document are sized from the text, so
`font-size` makes all of them smaller or larger together and fits more or less on a page; the
title row above the document keeps its own size.

A margin is one length for every side, two for the top and bottom then the sides, three for
the top, the sides and the bottom, or four from the top clockwise: `margin:20mm`,
`margin:25mm 20mm`. The header and the footer are printed inside the top and bottom margins,
so a margin too small to hold them hides them.

Each setting is taken from the last of these that names it: the default, the configuration
file, the document's front matter, the command line. A document can so carry its own page,
and the command line still stands above it:

```markdown
---
title: Installation guide
pdf:
  margin: 25mm 20mm
  header: title
---
```

```sh
serve-markdown --pdf-page footer:none --pdf guide.pdf docs/guide.md
```

A page break is written into the document as an element of the class `page-break`, which
starts a new page after it:

```markdown
<div class="page-break"></div>
```

Printed, a heading stays on the page of the text below it, and a code block, a table, a
picture, a diagram or a quotation is kept whole on one page unless it is longer than a page.

Chrome or Chromium must be installed where the server runs, and is found where
[chromedp](https://github.com/chromedp/chromedp) looks for it. The container image carries
no browser, so it does not print.

## macOS menu bar app

`./build.mjs macos` builds `dist/ServeMarkdown.app`, a menu bar app for macOS 13 or later that
starts serve-markdown on a folder or a Markdown file and keeps it running without a terminal.
It shows an icon in the menu bar, and no Dock icon or window. It needs the Command Line Tools'
Swift (`xcode-select --install`), not Xcode, and is built locally only.

The app holds its own serve-markdown, built from the same sources, and is signed with the
hardened runtime by the identity in `MACOS_SIGN_IDENTITY`, ad-hoc when it is not set.

```sh
./build.mjs macos
cp -R dist/ServeMarkdown.app ~/Applications/
open ~/Applications/ServeMarkdown.app
```

The menu holds:

- **Serve Folder or File...** opens the system open dialog for a folder or a `.md` /
  `.markdown` file and serves it;
- **Serve Recent** lists the last five folders and files served, the latest first, and serves
  the one chosen again, with or without the editor as **Enable Editor** says; **Clear Menu**
  empties the list;
- **Enable Editor** starts the next instance with `--editor` when ticked, and without it
  otherwise;
- one submenu for each running instance, named `<name> - :<port>`, with **Open in Browser**,
  **Open Editor** for an instance with the editor, **Show Log** and **Stop**;
- **Stop All**, while any instance runs;
- **Launch at Login** registers the app as a login item, which macOS may ask to approve in
  System Settings > General > Login Items;
- **About Serve Markdown** shows the app's version;
- **Quit Serve Markdown**.

An instance runs `serve-markdown --listen-address 127.0.0.1 --port <port> --editor=<true|false>
<path>` in the served folder, or in the folder of the served file, so it reads the
`serve-markdown.yaml` there as a run from a terminal in that folder would, else
`~/.config/serve-markdown/config.yaml`. The app picks a free port itself, and the address, the
port and the editor are always written on the command line, so they stand above any
configuration file.

The output of each instance goes to `~/Library/Logs/ServeMarkdown/<name>-<port>.log`, written
anew at each start; **Show Log** opens it in Console. An instance that exits unasked, such as
on a configuration error, is reported with the last lines of its log.

**Stop** interrupts the instance, as Ctrl-C does, and terminates it if it still runs 6 seconds
later. Quitting the app, logging out and shutting down stop every instance the same way. An
instance outlives the app only when the app itself crashes.

A login item registered by one build may need **Launch at Login** turned off and on again after
the app is rebuilt or moved. The app is registered from where it runs, so it is copied to
`~/Applications` before the option is turned on.

## Licence

MIT, see [LICENSE](LICENSE).
