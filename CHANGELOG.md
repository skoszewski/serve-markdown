# Changelog

## 3.4.0

feature: the menu follows its file and the documents' titles, and is drawn with a title and headings.

- A menu entry may name a document alone, `- docs/usage.md`. Its label is, in this order, the
  label written in the menu, the `title` of the document's front matter, the text of its first
  `# ` heading outside a code block, and its file name.
- The menu is read again whenever its file changes, and checked and labelled again on every
  page, a document's title read again once the document changes: a menu added to the file shows
  on the next page, and one removed from it brings the list back. A file that no longer reads
  keeps the menu it had, with a warning.
- A link to a document that is not served is warned of in the server's output once each time
  the warnings change, rather than in the startup banner alone.
- The first entry of the menu is its title, drawn bold and larger than any other entry.
- A menu group's label is drawn as a bold heading above its entries: largest for a group at the
  top of the menu, smaller for a group within it, and smaller again below that.

## 3.3.0

feature: a menu in the configuration file stands in place of the directory list.

- A `menu` in `serve-markdown.yaml`, or in the file `--config` names, defines the left pane: labels
  linking documents of the served directory, and groups of them nested to any depth. It stands
  on every page in the order written, the document on the page marked, and no folder is explored.
- The menu is checked at startup: an entry linking a missing document, a folder, a path outside
  the served directory, a dot-name or an ignored path is left out, as is a group left empty, each
  with a warning in the banner. An entry that does not read stops the server, naming its line.
- With a menu, `--list`, the `list` key and the `list` query parameter are not read; the banner
  warns when the command line or the file names them.
- A menu in the user's own configuration file, or for an `ado://` source, is ignored with a
  warning in the banner.

## 3.2.0

feature: a .serveignore file hides paths from the pages and the editor, and release.mjs makes releases.

- A `.serveignore` in the served directory holds `.gitignore` patterns of the paths neither listed,
  served nor edited: an ignored file or folder is left out of the directory list, a folder's list
  of Markdown files and the documents a folder is read as, and its URL answers `404 Not Found`.
- `--ignore-file <path>` (`ignore-file` in the configuration file) names the file to read in place
  of `.serveignore`. It must exist and hold only `.gitignore` patterns, or the server stops with a
  message naming the line; a `.serveignore` that does not read is a warning in the startup banner.
- The startup banner names the ignore file that was read.
- A URL reaching a file or folder whose name begins with a dot, `.serveignore` among them,
  answers `404 Not Found`, as the lists have always left such names out.
- `--editor-exclude` and `exclude` in `editor.json` take `.gitignore` patterns, `**` and `!`
  among them, read after those of the ignore file. A pattern ending in `/` matches folders alone.
- `CHANGELOG.md` records every release, and `./release.mjs` releases its newest entry: it runs
  `./build.mjs check`, commits on `dev` with the entry's summary line, tags `vx.y.z` with the
  entry as the release notes, pushes `dev` and `main`, and pushes the tag once their build has
  passed. `./build.mjs release` is removed.
- The release job refuses a tag that is not the version of the newest `CHANGELOG.md` entry, and
  publishes a prerelease version, such as `v3.2.0-rc.1`, as a GitHub prerelease.

## 3.1.4

update: the release's downloads carry the version in their names.

- The release's downloads carry the version, without the `v`, before the platform:
  `serve-markdown-3.1.4-<goos>-<goarch>.tar.gz` (`.zip` for Windows),
  `ServeMarkdown-3.1.4-darwin-arm64.zip` and `ServeMarkdown-3.1.4-windows-amd64.zip`.
- The `dev-latest` prerelease names its files with the dev version, such as
  `serve-markdown-3.1.4-dev-20261020-darwin-arm64.tar.gz`, so their download URLs change with
  every build. The download URLs without a version no longer exist.

## 3.1.3

feature: the macOS menu bar app is among the release's downloads.

- The macOS menu bar app is among the release's downloads, as `ServeMarkdown-darwin-arm64.zip`:
  `ServeMarkdown.app` with `serve-markdown` inside it, for Apple silicon. It is signed ad hoc,
  not with a Developer ID. The `dev-latest` prerelease carries it too.

## 3.1.1

feature: the Windows app is among the release's downloads, and the executables are packed.

- The Windows notification-area app is among the release's downloads, as
  `ServeMarkdown-windows-amd64.zip`: `ServeMarkdown.exe` with `serve-markdown.exe` beside it, for
  Windows x64. It is self-contained, carrying its own .NET runtime, so it is unzipped and run
  with nothing else installed. The `dev-latest` prerelease carries it too.
- The `serve-markdown` executables are published packed: `serve-markdown-<goos>-<goarch>.tar.gz`
  for Linux and macOS, and `serve-markdown-windows-<goarch>.zip` for Windows, each holding the
  executable under its plain name, `serve-markdown` or `serve-markdown.exe`. A tarball keeps the
  executable bit, so the command runs once unpacked.
- The download URLs of the bare executables, in releases and in `dev-latest` alike, no longer
  exist.

## 3.1.0

feature: --stop-on-stdin-eof, and menu bar and notification-area apps built from a clone.

- `--stop-on-stdin-eof` stops the server, as Ctrl-C does, once its standard input ends: the
  server prints "Stopping server" and gives the requests in flight up to 5 seconds. It is read
  from the command line alone.
- A macOS menu bar app, built from a clone with `./build.mjs macos` into
  `dist/ServeMarkdown.app`: it serves a folder or a Markdown file chosen in the open dialog,
  optionally with the editor, lists the running instances with their logs, serves recent
  folders again, and can start at login.
- A Windows notification-area app, built from a clone with `./build.mjs windows` into
  `dist/ServeMarkdown-windows`, framework-dependent or, with `self-contained`, carrying the .NET
  runtime. It offers the same menu and stops its instances through `--stop-on-stdin-eof`.
- The two apps are not among the release's downloads; the release carries the `serve-markdown`
  executables as before.

## 3.0.1

feature: --editor-config names the editor configuration file, and a bad editor.json is a warning.

- `--editor-config <path>` (`editor-config` in the configuration file) names the editor
  configuration file to read in place of `editor.json` in the served directory. The file must
  exist and read, or the server stops with a message naming it; the flag needs `--editor`.
- A missing `editor.json`, or one that does not read - an unknown key, a folder outside the
  served directory, or a pattern that does not read - is a warning in the startup banner instead
  of an error. The file is set aside and the editor edits the whole served directory, but for
  what `--editor-exclude` names.

## 3.0.0

feature: --editor serves a visual Markdown editor of the served directory.

- `--editor` serves a visual Markdown editor of the served directory at `/_/editor/`, built on
  MDXEditor: a file list, the document with a formatting toolbar, source and diff modes, an
  outline, and the document properties - title, author, tags, created and updated - kept in the
  front matter, the rest of which is written back as it was.
- New documents are created empty, from the templates of `--editor-templates`, or as a folder
  with an `index.md`; the preview opens the page the server renders of the saved file.
- `editor.json` in the served directory names the folders whose documents are edited and the
  paths left out; `--editor-exclude` adds patterns for one run.
- Documents are read as plain Markdown, their HTML kept as written, or as MDX, chosen in the
  settings.
- Colour themes, each light and dark: GitHub, the default, Visual Studio Code, Solarized,
  Catppuccin, Gruvbox and Monochrome, with a theme button sharing its choice with the pages.
- Optional Git controls: branch, fetch, pull, commit of the changed documents, and push.
- Every push to `dev` publishes `darwin/arm64` and `windows/amd64` builds as the `dev-latest`
  prerelease, named `vX.Y.Z-dev-YYYYMMDD`.
- The Go module path is `github.com/skoszewski/serve-markdown/v3`.

## 2.3.1

update: every page names the server and its version.

- Every page names the server and its version, `Serve Markdown v2.3.1`, in small grey text to the
  left of the control buttons. It is left out of a printed page.

## 2.3.0

feature: --css styles every document whose front matter declares no css of its own.

- `--css`, written in the configuration file as `css`, styles every document whose front matter
  declares no `css` of its own, a document without front matter among them. A document's own
  `css` replaces it rather than adding to it. It applies to `--pdf` too.

## 2.2.1

update: the startup banner names the version.

- The startup banner names the version: `Serve Markdown v2.2.1`.

## 2.2.0

feature: --pdf-page takes font-size, the size of the document's text in a PDF.

- `--pdf-page` takes `font-size`: the size of the document's text in a PDF, in `pt` or `px`,
  e.g. `font-size:11pt`. The headings, the code and the tables are sized from it, so a smaller
  size fits more on a page. Like the other page settings it can be written on the command line,
  as `pdf-page` in the configuration file, or under `pdf` in a document's front matter. Unset,
  the stylesheet's own 16px stands.

## 2.1.0

feature: --pdf-page takes margins, a header and a footer, and documents name their own page.

- `--pdf-page` takes `margin`: 1 to 4 lengths in `mm`, `cm`, `in` or `pt`, written as CSS writes
  a margin, e.g. `margin:25mm 20mm`.
- `--pdf-page` takes `header` (`none`, `title`) and `footer` (`none`, `page`, `pages`): the
  document's title at the top of every page, and its page number, alone or of the number of
  pages, at the bottom.
- A document can name its own page in its front matter, under `pdf`, with the same settings.
  Each setting is taken from the last of the default, the configuration file, the front matter
  and the command line that names it.
- Page breaks: `<div class="page-break"></div>` in the Markdown starts a new page after it.
- Printed, a heading stays with the text below it, and a code block, a table, a picture, a
  diagram or a quotation is kept whole on one page unless it is longer than a page.
- A PDF is printed with 20 mm margins and a `N / M` page footer unless told otherwise; 2.0.0
  printed 0.4 in margins and no footer. `--pdf-page footer:none,margin:0.4in` prints as before.

## 2.0.0

feature: --pdf prints a document to a PDF, and the browser libraries are built into the binary.

- `--pdf <file.pdf>` prints one Markdown document (local or `ado://`) to a PDF with headless
  Chrome and exits. The PDF matches the page: title row from the front matter, highlighted code,
  mermaid diagrams with `--mermaid`, and bookmarks from the headings. It needs Chrome or
  Chromium installed; the container image has none.
- `--pdf-page` sets the paper (`a3`, `a4`, `a5`, `letter`, `legal`, `tabloid`) and the
  orientation (`portrait`, `landscape`); also `pdf-page` in the configuration file.
- Theme button: follow the browser, light, or dark; remembered per server.
- List button: the directory list on the left can be hidden and shown like the outline.
- `--list` takes `display:shown|hidden`, in the configuration file and the `?list=` query too.
- PowerShell highlighting (`powershell`, `pwsh`, `ps`, `ps1`).
- Print styles: a page printed from the browser leaves out the buttons and sidebars.
- Browser libraries are built from pinned npm packages with esbuild: marked 18, highlight.js
  11.12, DOMPurify 3.4, mermaid 12.1, github-markdown-css 5.9.
- `./build.mjs` replaces `build.sh` and `build.ps1`, and adds the `assets` and `update` actions.
  Building from source needs Go and Node.js.
- CI checks every push to `dev`; releases are built only from semver tags on `main`.
- The module path is `github.com/skoszewski/serve-markdown/v2`.
- `--online` is removed: everything the page loads comes from the binary.
- `go install` is no longer the way to install: download a release or build from a clone with
  `./build.mjs`.

## 1.0.1

feature: --debug turns on debugging output.

- `--debug <level>` turns on debugging output for a level above 0.

## 1.0.0

update: releases are made from semver tags alone.

- Releases are built from semver tags, which the build workflow validates.
