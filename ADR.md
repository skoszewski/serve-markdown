# Architectural Decision Records

## The page shell lives in embedded files rather than in Go literals

The shell was two Go string constants assembled with `fmt.Sprintf`, whose positional
arguments had to be kept in step by hand. The outline and the diagram support roughly tripled
the script.

The shell is now `assets/page/page.html` and `page.css`, embedded with `go:embed`, and the
script built from `web/src/page.js`. The HTML is rendered with `html/template` and the
settings the script needs are written into the page as one `pageConfig` object, which
`html/template` escapes for a script context. The styling is served under `/_/page/` and the
script with the other built assets under `/_/assets/`, and the browser caches them between
page loads.

## The outline is numbered in CSS, not in JavaScript

The script only nests the lists, following the heading levels; each `--outline` style is a
class on the outline, and the numbers come from CSS counters. `numbered-hierarchical` is
`counters(section, ".")`, which writes `1.`, `1.1.`, `1.1.1.` without the script knowing the
numbering at all, so a style is added by writing a rule.

## Mermaid is the UMD bundle, loaded on first use

Mermaid publishes an ES module that pulls its diagram types from a `chunks` directory of some
twenty megabytes, and a single self-contained `dist/mermaid.min.js` that sets
`globalThis.mermaid`. The single file is copied into the built assets, so one more file keeps
the server free of a network of its own, at 5.5 MB of the binary.

It is injected by the script the first time a document holds a `mermaid` block, rather than
loaded by the page shell, so that a document without diagrams does not carry it. `--mermaid`
guards the whole of it: without the flag the page holds no mermaid URL, and a `mermaid` block
stays a highlighted code block.

## The outline is described by a settings list

`--outline` takes a comma separated list of `key:value` pairs - `style:plain,justify:right` -
rather than one value, since the outline has more than one thing to say about it and a flag
for each would multiply as the page grows. The list is read by `parseSettings`, which passes
each value to the reader its key names, so another flag can be described the same way.

A key left out keeps what it had, which is what lets a page's `outline` query parameter carry
the same list and apply it onto the server's own settings for that page alone.

## One tree walk reads both kinds of source

A local source reads a filesystem and an `ado://` source reads a repository over REST, but
both are a tree of folders and documents. `documentTree`
holds the whole of the building - the scope, the nesting, the ordering, the `..` entry, the
marking of the document on the page - and asks a `treeProvider` for the folder it starts at,
what a folder holds, and the route that addresses an item. `localTree` and `adoTree` are those
providers, and a third kind of source would only have to answer the same questions.

## The document says what its links are read from

A document's relative links are resolved by the browser against the page's own address, which
drops its last segment. A folder served at `/_/ado/<org>/<project>/<repo>` therefore turned
`pool/README.md` into `/_/ado/<org>/<project>/pool/README.md`, reading the repository name as
a project's, and Azure DevOps answered TF401019; a local folder at `/docs` lost its last
segment the same way.

The route is not the place to fix it. An `ado://` route is
`/_/ado/<organization>/<project>/<repository><path>`, whose first five segments are the
address of the repository itself and are not the server's to rewrite, and a folder is a route
the reader may write with or without a trailing slash either way.

So `/content` answers with the base the links are read from - the route of the folder the
document was resolved in, which only the server knows, since a route naming a folder resolves
to a document inside it - and the page resolves the document's own links and pictures against
it. Nothing redirects, and every shape of route is served as written.

A `<base>` element would do the same for the browser, but it also moves every anchor on the
page, which would send the outline's links away from the document instead of down it.

## Browsing carries the page's query

The outline and the list settings live in the query, so a link that dropped it would take the
reader to a page drawn differently from the one they were reading. Every list entry is given
the request's own query, and the page script gives it to each link of the rendered document
that points back at the server and carries no query of its own.

## An unread outline query leaves the server's settings

A query parameter that does not read is ignored rather than answered with an error: the page
is a document to read, and a mistyped parameter should still show it. `--outline` is the
opposite - it stops the server - since a flag is written once and a mistake in it should be
told at the start rather than left to be noticed in the page.

## The shell's elements are named with a leading underscore

The outline links to headings by an anchor slugged from their text, and a document with an
"Outline" heading took the id of the outline itself, which then drew the heading as the
sidebar. A slug holds letters, digits and hyphens alone, so the shell's own elements are
`_outline`, `_content` and `_document-css`, which no heading can be given.

## Pictures are served as their own bytes, by suffix

Every route but `/content` used to answer with the page shell, so an `![](diagram.png)` in a
document fetched an HTML page and rendered as a broken image. A route whose suffix names a
picture or another binary file is now answered with the file's bytes instead. The suffix
decides it, since the alternative - a route namespace of its own - would mean rewriting every
relative link in the rendered document.

The responses carry `Content-Security-Policy: sandbox`, so that an SVG opened as a page of its
own cannot run the script it may hold.

An Azure Repos picture is read as the item's own stream rather than through the JSON answer
the documents are read with, since a binary file does not survive being carried as a JSON
string.

## A folder's document is named by one list, the same for both sources

`--search` names the documents a folder is read as, looked through in the order written, and
`README.md` then `index.md` is the default, that being the name a folder's document usually
carries. The list is one for both sources rather than one per scheme: a folder resolves to the
same document however it is reached, and a repository whose folders are written another way is
served by naming that way once.

`--index-only` stops the search at those names: a folder is the document it holds or a page
saying none was found, and the listing of whatever else stands there is left out.

Within an Azure Repos repository the list beside the page follows it as well, naming the
folders and their index documents alone: an entry the page would never open leads nowhere,
and counting such entries kept a repository root from falling back to the project's
repositories when nothing else was there to browse. A folder stands for one document at most,
the one it is read as, so a folder holding both an `index.md` and a `README.md` is listed once
rather than twice.

## The document's width is named, and the names are measured

A fixed 980px column - `github-markdown-css`'s own - left most of a large screen empty, the
more so with the two 320px sidebars beside it. `--content-width` names four widths rather than
taking a number, so that a configuration file reads as a choice rather than a measurement, and
each name is anchored to something: 780px is prose at about 86 characters, 980px is what the
styling was written for, 1280px is a 1080p window less both sidebars, and `full` is the window
itself.

`full` is what a page is drawn at unasked, since a reader who has not said otherwise is better
served by their whole screen than by a third of it. It carries no class on the page, being the
absence of a cap rather than a cap of its own.

## One file says what the server was told

`config.go` is where the settings are named, defaulted, read and decided between: the flags
and their help, the defaults each falls back to, the file's keys, and the rule that a flag
written on the command line stands above the file. It answers with one `configuration`, which
the rest of the server reads; nothing else opens a file or looks at a flag to find out what to
do, and no default is written twice.

The seconds between the page's checks are asked of it rather than computed where the server is
built, since the default depends on the kind of source: `watchInterval(kind)` is the
configuration's own answer.

## A configuration file is the flags, written down

The file carries one key per flag, named as the flag is, so there is nothing to learn twice
and nothing to keep in step but the names. A flag taking a settings list is a mapping of those
settings, or `true` for its own, and each setting is handed to the flag's own reader - a file
is refused for the same reasons a command line is, with the same words.

`--css` is a flag for that reason too, though CSS is mostly written in a file: a document's
default styling is a setting like any other, and a key no flag is named after would be one
more thing to learn. A document's own front matter `css` replaces it rather than adding to
it, so a document styled for itself keeps exactly the styling it was written with.

The command line stands above the file: a file belongs to a directory and is read by everyone
who serves it, while a flag belongs to one run. Which flags were written is read from the flag
package itself rather than by comparing against defaults, so writing a flag's own default
still counts as having written it.

Unknown keys are refused rather than passed over. A file that quietly ignores `outlyne: true`
teaches the reader that the setting does not work, and says nothing about why.

The file is looked for where the server is run from, not beside the path it serves, and then
in the user's configuration directory as `os.UserConfigDir` names it for the platform. On
macOS `$HOME/.config` is looked in before it, since a tool used on Linux and macOS alike keeps
its configuration there on both. The
first file found is read alone; the files are not merged, so what a run is told can be read
from one file.

## The version is chosen on the page, from lists the page already holds

A repository read at a branch or a tag was reachable only by writing `?ado=branch:...` into
the address, which means knowing the branch names by heart. The page now draws the choice
above the document: the default branch in a box of its own, since it answers "what am I
reading?" without a list being opened, then Branch and Tag as one mutually exclusive pair, and
the names of whichever is chosen.

Both lists travel with the page rather than being fetched when a button is pressed. They are
small, they are already read and kept for half an hour, and a picker that asks the server
before it can show its options is a picker that feels broken on a slow link. Choosing a name
writes the `ado` parameter into the address and opens the same document there, so what the
picker does and what a typed URL does are the same thing.

## A personal access token stands before the az CLI

Reading Azure Repos through `az account get-access-token` asks that the Azure CLI is installed
and signed in, which a container image built from a static binary, a CI job and a server
without a browser to sign in with all lack. An environment variable is what those places
already carry a credential in, so `AZURE_DEVOPS_PAT` is read first and the CLI is the
fallback. That name is the one the REST API documentation writes, and belongs to no particular
tool, where `AZURE_DEVOPS_EXT_PAT` is the `az devops` extension's own.

The token is carried as basic authentication under an empty user name - `Basic` of
`:{token}` - which is how Azure DevOps takes a PAT, while the CLI's token is a bearer. Since
the two differ in more than their value, what is built is the whole header rather than a token
the caller must know the scheme of.

The variable is read on every request rather than kept, because it costs nothing to read and a
token replaced in the environment of a restarted server should not have a stale copy standing
against it. The CLI's token keeps its half hour, as acquiring it costs a process.

## What an organization, a project and a repository hold is kept for half an hour

Every page below a repository draws a sidebar from the organization's projects, and often from
a project's repositories too, while a page reading a repository draws its picker from the
branches and tags; so browsing asked Azure DevOps the same few questions again on every page.
Those answers change when someone creates or deletes a project, a repository, a branch or a
tag - rarely, and never while a document is being written - so they are kept under their names
for `adoTokenLifetime`, the half hour the access token is already kept for. One
age is easier to hold in mind than two, and both are about the same thing: how stale the
server is willing to be.

A repository's own documents are not kept: they change with every push, which is what the
reader is there to see. A read that fails is not kept either, so a listing refused while a
token is expiring is asked for again rather than remembered as empty.

## An ado:// page is read on refresh, not on a timer

A local file changes while it is being written, which is what the page's checks are for: an
editor and a browser side by side. A repository changes when someone pushes to it, and every
check costs a REST request - for the document, and for the list beside it - against a quota
that is not the reader's alone.

So a page reading Azure Repos makes no checks: it is read once, and read again when the
browser is refreshed, which builds the sidebar afresh as well. `--watch-interval` keeps its
meaning for local pages, and says nothing for these.

## The version a repository is read at travels with the source

Azure Repos reads an item at a version, defaulting to the repository's branch. `--ado` names
one as `branch:`, `tag:` or `commit:` - three keys rather than one carrying the kind, so the
list says what it means - and a page's `ado` parameter replaces it for that page, the way the
outline and list parameters replace theirs.

The version is carried on the source rather than on the server, since one request may read at
a version another does not: it is set once where the route is resolved and reaches the item
requests through the query they build. The listings of projects and repositories have no
version, so nothing is added to them.

## An organization and a project are folders

Azure DevOps nests an organization, a project, a repository and then the files, so the two
levels above a repository are folders the way a directory is - except that they can hold no
`README.md` to be read instead. Each is therefore served as the listing of what it holds, the
way a local directory holding no document is, and the route below an entry opens it.

The listings have no modification time or object ID to mark a version with, so their change
marker is a hash of the text they render to: the page re-renders when the list of projects or
repositories differs, and stays still while it does not.

The sidebar carries the organization's projects above a repository, the one on the page
marked, so that moving between projects takes one click; a repository carries its own
documents under a link back to the repositories of its project, which is the one step the file
tree cannot express. Without that link a reader who had walked down into a repository could
only leave it by editing the address.

The `scope` reaches below those entries the way it does within a repository: `subfolders`
nests the repositories of the project on the page, and `tree` those of every project, which
costs one read per project and is asked for rather than assumed.

A repository holding its index document and nothing else would list that one document, which
is the document the reader is already looking at. The list is then the project's repositories, the
one on the page marked, so the sidebar always offers somewhere to go. The link above them
leads to the project as it always does, under a label naming what the list now holds.

That swap belongs to the repository's root alone. A folder within a repository keeps its own
list however short it is, since the folder above it is a step the reader will want, and the
link above the list is the one that takes it: **Up** below the root, **Browse to
repositories** at it.

Telling that case apart takes more than the entry's mark: a repository's own route addresses
the document inside it without naming it, so nothing in the list is marked. An only entry that
`--search` names is therefore read as the document on the page, while an only entry under
another name is kept - the page did not resolve to it, and that entry is the only
way to reach it.

## A local file has one route, and ado:// its own namespace

A local document is served at `/<path>` alone. Naming the source in the route as well - a
namespace resolving a directory to its document, another listing the directory instead - gave
one document several addresses answering the same question, and a link followed from one of
them landed in another. The directory's own document, the listing when it holds none, and the
page saying it holds no Markdown at all are three answers to one question, so they are one
route with a fallback rather than a route each.

An `ado://` source is the exception, and not served at the root: the repository is named by
`/_/ado/<organization>/<project>/<repository>`, so serving it at `/` as well gave the same
document two addresses, one of which dropped the part saying where it comes from, and a
relative link from the rooted one resolved into the wrong repository. A local path below the
server's own directory needs no such naming, the rest of the route already saying it.

## Hiding the outline is the reader's choice, kept by the browser

The outline is hidden and shown by a button on the page rather than by the server: it is a
reader's preference while reading, not a setting of what is served, and `--outline` and the
`outline` query parameter already say whether the page carries one at all. Every link opens a
new page, so the choice is kept in `localStorage`, which the browser holds per server origin,
and read by the inline script in the head, before the page is first drawn, so that a hidden
outline does not appear and then fold away.

The button stands in a top row above the sidebars and the document, whose right is kept for
the page's control buttons, so hiding the outline removes it whole and leaves the button where
it was. The row is on every page, whether it holds a button or not, so that the layout does
not move between pages. Its height follows the document's title and the line under it, so
the page script measures the row and hands the height to the stylesheet, which the sticky
sidebars and the scroll to a heading stand below. The button is an icon drawn inline in the page, so it needs no font or
file of its own; the stylesheet empties its panel and mirrors it from the same classes that
hide and place the outline, so the icon and the outline cannot disagree.

The outline's `display` setting says how it starts, and the reader's choice stands above it:
the server writes `outlineHidden` into the page, and the head script reads it only when the
browser holds no choice. A setting that overrode the reader would undo the button on every
page they open. The directory list's `display` follows the same rule, its choice kept apart
from the outline's, so hiding one leaves the other as the reader left it.

## The theme is chosen by switching stylesheets

The page could only follow the browser's light or dark preference, read by `media` queries in
its stylesheets. A reader can now choose light or dark over it. Every stylesheet that differs
between the two comes as a light and a dark file - github-markdown-css's own pair,
highlight.js's GitHub pair, and `page-dark.css` over `page.css` - each link marked with the
theme it draws. The head script sets their `media`: the preference query for the browser's
theme, `all` for the theme chosen and `not all` for the other, so one rule decides the whole
page and nothing is drawn twice over.

The choice is kept in `localStorage` and applied by the head script before the page is first
drawn, as the sidebars' are, so a dark page does not flash light. Mermaid draws its theme into
the diagram itself, so choosing a theme draws the document again. The server holds no setting
for it: the theme is the reader's alone.

## The GitHub workflow builds on its own

The workflow runs `gofmt`, `go vet`, the tests and the builds with its own steps rather than
through `build.mjs`, so the script serves a developer's machine and the workflow serves GitHub,
and neither changes for the other.

Each platform's executable is published packed, as
`serve-markdown-<version>-<goos>-<goarch>.tar.gz`, or `.zip` for Windows, the form each platform
unpacks without extra tools, holding the executable under its plain name, `serve-markdown` or
`serve-markdown.exe`. A download keeps the executable bit of a tarball, so it runs once
unpacked, where a bare executable downloaded from a release has to be marked executable first.
The archive is uploaded as the artifact without a further zip around it (`archive: false`), so
a run's artifact downloads as the archive itself. Every archive and app zip carries its version
without the `v` - the tag's on a release, the dev version on `dev` - before the platform, so a
downloaded file tells which build it is.

The release is made with the `gh` CLI the runner already carries, rather than with a
third-party action, and only for a tag that is already pushed. Its notes are the message of the
annotated tag, the newest `CHANGELOG.md` entry (see "A release is the newest CHANGELOG.md
entry, made by release.mjs"). The history is checked out whole for the builds, so that Go reads
the version from the tag the commit carries.

A release is made for a semver tag with a leading `v` alone, the form Go reads a module
version in. The workflow's tag filters are globs, which hold a tag to `vMAJOR.MINOR.PATCH` with
an optional pre-release but cannot refuse leading zeros or empty identifiers, so the release
job holds the tag to the semver grammar before publishing. Build metadata is refused, since Go
does not take it in a version.

A push to `dev` is built for `darwin/arm64` and `windows/amd64` alone, the platforms it is
tried on, and published as one prerelease under the fixed tag `dev-latest`, so its release
page never moves. The tag is not `dev`, which would make the name ambiguous with the branch.
The prerelease is deleted with its tag and made again rather than moved, so the tag always
points at the commit built, and it is never marked Latest. A dev build is given its version
at link time with `-X main.buildVersion`, as `vX.Y.Z-dev-YYYYMMDD` from the latest release tag
and the UTC build date, since the pseudo-version Go stamps on an untagged commit names the
next patch release and does not read as a dev build.

A release is refused for a tag whose commit is not on `main`, since `main` holds the stable
code and releases are made from it.

## A release is the newest CHANGELOG.md entry, made by release.mjs

`CHANGELOG.md` is the one record of what each release changed, so a release takes its commit
message, its tag and its notes from the newest entry rather than from anything written at
release time: the summary line is the commit message, the heading the version, and the whole
entry the message of the annotated tag, which the workflow publishes as the release notes. The
tag is written with `--cleanup=whitespace`, so lines starting with `#` stay in the notes. The
release job refuses a tag that is not the version of the newest entry, so a tag made by hand
does not publish a release the changelog does not hold, and publishes a prerelease version as a
GitHub prerelease, which is never marked Latest.

`./release.mjs` is the only way a release is made. It moves `main` with `git fetch . dev:main`,
so the working tree stays on `dev`, and pushes `dev` and `main` before the tag, waiting for the
build of the `dev` push to pass: a pushed tag publishes a release that cannot be taken back,
while a failed branch build costs only a commit, the tag still being on the local machine. A
tag is `v` and a semver version, the form Go reads a module version in, so an entry is
`x.y.z` or a semver prerelease; a later release of the same code takes the next patch version,
since `x.y.z-n` would be a prerelease ordered before `x.y.z`.

A run also builds the desktop apps, each on its own platform; see "The apps are built on their
own platforms by the workflow".

## The browser assets are built from pinned packages

The libraries were stock builds downloaded from CDNs, which left their configuration to
whoever published them: highlight.js's common build has no PowerShell, and adding a language
meant another file and another script tag. They are now npm packages pinned by
`web/package-lock.json` and built by esbuild, so what goes into the page is chosen here.

The page script imports marked, DOMPurify and highlight.js and is bundled with them into one
file, so the page loads one script and the libraries are never missing. highlight.js is built
from its core with the languages `web/src/highlight.js` registers. Mermaid stays a file of its
own, loaded on first use for the reasons given above.

The built files are not kept in the repository, so they cannot fall out of step with the
sources and the lockfile; every build makes them first. That costs `go install`, which builds
from the module alone and runs no build of its own: the server is installed from a release or
built from a clone, with Node.js beside Go.

The CDN option, `--online`, is gone: it could only load the stock builds, not the ones built
here, and the server needs no network for its page.

`build.mjs` replaces the shell and PowerShell scripts as the one build script for every
platform. Node.js is needed for the assets anyway, and one script cannot drift from another.

## A folder leading nowhere is listed as the folder above

A list holding the document already on the page and `..` gives the reader nothing to choose.
A local folder below the served directory whose list leads nowhere is listed as the folder
above instead, the folder itself marked, the way a repository root leading nowhere carries its
project's repositories. One check, `holdsNothingToBrowse`, decides both.

The folder above is listed with `subfolders` whatever scope was asked for, since the folder on
the page is one of its entries and `current` leaves folders out. The served directory has
nothing above it the server may show, so it keeps its own list, and `tree` already lists
everything.

## A PDF is printed by headless Chrome from the page itself

The document is rendered in the browser - marked, highlight.js, mermaid - so a PDF made by a
Go library from the Markdown would draw it differently and leave the diagrams out. `--pdf`
prints the page instead: the server serves it on a loopback port of its own, headless Chrome
draws it and prints it with `Page.printToPDF`, and the server stops. The PDF is then what the
reader sees, from one renderer.

Chrome is driven with chromedp rather than run as a command printing a URL, since the page
draws itself after loading - it fetches the document, then its diagrams and pictures - and
only a driver can wait for that. The page marks itself drawn once all of it is, and the export
waits for the mark.

The page knows it is printed from `pageConfig.print`: it takes the light theme whatever the
reader chose in the browser, and keeps a link into the server as text, the server being gone
once the PDF is written. What it leaves out - the buttons, the sidebars, the version picker -
is left out by print styles, so the same rules hold when a reader prints a page from the
browser. No browser is bundled: chromedp finds the one installed, and the container image,
built for serving, carries none.

## The printed page follows md-to-pdf, written as settings

The page a PDF is printed on - its margins, header, footer and page breaks - follows what
md-to-pdf offers, the tool most often used for the same work, so that a document moved from
one to the other prints alike. Its settings are written the way the rest of the server's are,
rather than as md-to-pdf's Puppeteer options passed through.

The header and the footer are named, not written: `header:title`, `footer:pages`. md-to-pdf
takes Chrome's header and footer templates as raw HTML, which asks a document's author to know
Chrome's template classes and to size the text themselves, since Chrome draws it with no size
at all. The server holds the templates, writing an empty one for the side left out, since
Chrome prints its own date and address in a header it is not given.

A margin is written as CSS writes one, as md-to-pdf takes it, in the units a page is measured
in. A document's front matter may name its own page under `pdf`, standing above the
configuration file and below the command line, the order md-to-pdf merges its own front matter
in; each setting is taken separately, so the command line can change one without restating the
rest. The page is laid together only once the document is read, and the file and the command
line are read at start, so a setting neither may say stops the server before a browser starts.

A page break is a class, `page-break`, as in md-to-pdf, written as an HTML element in the
Markdown, since Markdown has no syntax of its own for one and a rule or a heading already
means something else. The print styles also keep a heading with the text below it and a block
whole on one page, which md-to-pdf leaves to the stylesheet.

The size of the printed text is a setting of its own, `font-size`, rather than CSS a document
writes for itself, since fitting more on a page is the commonest change made to a printed
document and the selector it takes is easy to get wrong: github-markdown-css sets the size on
`.markdown-body`, and a bare `body` rule loses to it. The size is written on that same rule in
the printed page, and everything the stylesheet sizes in `em` follows it. It takes `pt` and
`px` alone, the units a printed page and the stylesheet are measured in, and is left out of
the page when it is not named, so the stylesheet's own size stands.

## The editor is a React application served by the server itself

The editor is a single page application built on MDXEditor, a React component with no
counterpart outside React. Its sources are TypeScript
in `web/src/editor/`, bundled by the same esbuild step as the page script, type checked by `tsc`
before it, and embedded in the binary with the other built assets. Its backend is the Go server:
the API under `/_/editor/api/`, the shell in `assets/page/editor.html`.

Serving it from the same server as the pages lets the preview open the page the server renders
of the file, with no address of another server to configure, and the pictures of a document are
read from the routes that already serve them.

## The editor is off unless asked for, and edits the served directory

The server reads files; the editor writes them, commits and pushes, so it takes `--editor`. It
edits below the directory served, since the server is already pointed at one and a path outside
it is refused everywhere else.

Which folders below it are edited and what is left out belong to the documents rather than to
the server's run, so they are kept beside them in `editor.json`, as the editor this one follows
kept them: `directories`, relative to the served directory and the whole of it when left out,
and `exclude`. `--editor-exclude` adds patterns for one run. Both are `.gitignore` patterns
added after those of the ignore file, to one matcher (see "Ignored files follow .gitignore,
through a library"), so the editor leaves out what the viewer does and its own paths besides,
and a `!` of its own can take back, for the editor, a path the ignore file leaves out.

A missing `editor.json`, or one that does not read, is a warning in the banner rather than a
failure: the server is already serving, and a file that only narrows what is edited is not worth
refusing to start for. The file is then set aside whole rather than read in part, so the editor
never edits under exclusions the author did not fully write.

`--editor-config` is the opposite: a path written on purpose is not guessed at. The file it names
must exist and read, or the server stops, since a warning would leave the editor open on
documents the reader meant to hold back.

A request changing a file is answered only without an `Origin` of another host and with a JSON
body. A page of another site can send a form to the loopback address, but neither JSON nor its
own origin past those checks, and the listen address stays `127.0.0.1` by default.

## Ignored files follow .gitignore, through a library

What a directory keeps out of sight belongs to the directory, as its `.gitignore` does, so the
patterns are a file in it, `.serveignore`, rather than a list in the server's configuration;
`--ignore-file` names another for a directory that cannot hold one. A file of its own, rather
than the directory's `.gitignore`, since what is kept out of Git and what is kept from readers
differ: a generated page is ignored by Git and meant to be read.

The patterns are read as Git reads them - `**`, `!`, anchoring, folders alone, a path below an
ignored folder staying ignored - which is a grammar with corners best left to code tested against
Git's own. `github.com/git-pkgs/gitignore` follows Git's `wildmatch.c`, is tested against its
suite, imports the standard library alone, and reports a pattern that does not read with its
line. Its matcher is made with an empty root, which reads no file of its own: the user's global
excludes and `.git/info/exclude` would hide paths the served directory never asked to hide.

An ignored path is hidden and not served alike: a list leaving a file out while its URL still
answered would only hide it from someone who did not know its name. The check stands in
`resolveRoute`, through which every local page, its content, a picture and the list beside the
page are found, and it is made on the route as written and on the path it resolves to, so a
symbolic link does not reach an ignored file under another name. A name beginning with a dot is
refused there the same way, the lists having always left it out, so `.serveignore` does not tell
a reader what it hides. A file named as the path is served as named, the server having been
pointed at it.

A `.serveignore` that is missing ignores nothing, and one that does not read is a warning and set
aside, as `editor.json` is; the file `--ignore-file` names must read, as `--editor-config`'s
must, since a warning would leave served what was meant to be held back. The file is read once,
at startup, as the editor's configuration is.

## The front matter is edited as a YAML node tree

A document's front matter carries keys the editor does not know - `css`, `pdf`, a site's own -
and comments. The editor reads it into a `yaml.Node` mapping and changes only the keys it owns,
so the rest is written back in its order, with its comments and its styles. `created` and
`updated` are written as YAML timestamps in UTC seconds.

## The editor's colour themes are CSS variables over published palettes

The themes are GitHub's Primer, Visual Studio Code's classic Light+ and Dark+, Solarized,
Catppuccin (Latte for light, Mocha for dark), Gruvbox and the Gray scale of Radix Colors, each an
MIT-licensed palette with a light and a dark variant. GitHub is the default, since its page and
code colours are those of github-markdown-css, which draws the pages the server renders, and its
top bar is GitHub's own header. Light+ and Dark+ keep the white page under the blue status bar
VS Code is known by. Radix Gray is a neutral scale with no tint, drawing the monochrome theme,
whose accent, errors and code are greys of the same scale. Each sets one list of variables on the
root element, chosen by its `data-palette` and `data-mode`; the rules and MDXEditor's own
variables read them, so a theme is added by one block per variant.

The code blocks of MDXEditor carry a light CodeMirror theme it always adds. The editor adds a
highlighter emitting fixed `cm-t-*` classes and colours them, and the editor's chrome, with
selectors that outweigh that theme's single class rules, rather than replacing MDXEditor's
CodeMirror setup.

The appearance - the browser's preference, light or dark - is the one the pages' theme button
stores, so the editor and the pages agree.

## The editor edits plain Markdown

The toolbar offers what plain Markdown writes and the page renders: headings, formatting,
lists, links, pictures, tables, rules and code blocks. MDXEditor's directive and admonition
syntax (`:::note`) is left out, since the page's renderer does not draw it and a document would
show it as text.

## The editor reads plain Markdown unless told to read MDX

MDXEditor reads a document as MDX: it turns CommonMark's HTML and autolinks off and parses
every tag as JSX, which refuses `<br>`, unquoted attributes and `<https://...>` - Markdown the
pages render without complaint. The editor reads plain Markdown by default, with MDXEditor's
`suppressHtmlProcessing`, and MDX only when the setting asks for it.

MDXEditor then has no reader for the HTML it no longer parses, and refuses the document. The
editor adds one, `rawHtmlPlugin`, below MDXEditor's own readers of `<img>` and the formatting
tags: the HTML is kept in a node of its own, a block at the top level and inline elsewhere, shown
as its source and written back as it was read. Editing it belongs to source mode, since a form
of every HTML element is beyond the editor and the text is the document's own.

## The menu bar app is a Swift package assembled by build.mjs

The menu bar app is SwiftUI's `MenuBarExtra` in the `.menu` style, with `LSUIElement` keeping it
out of the Dock, and `SMAppService.mainApp` registering it as a login item. It is a Swift
package built by SwiftPM with the Command Line Tools, without an Xcode project, so the `.app`
bundle is assembled by `./build.mjs macos` rather than by Xcode: the executables, an
`Info.plist` written from `macos/Info.plist.in` with the version of the latest tag, and
`PkgInfo`. `build.mjs` stays the project's one build script.

The assembly and the signing follow `packaging/macos` of storytold/pdfcraft, which builds an
app bundle without Xcode the same way. The bundle is signed inside-out with the hardened
runtime and an empty set of entitlements - the bundled serve-markdown first, then the app,
without `--deep` - and verified with `codesign --verify --strict --deep`. The identity is
`MACOS_SIGN_IDENTITY`, ad-hoc by default, which needs no Apple Developer Program membership.

The SDK of the Command Line Tools declares SwiftUI's `@State` as a macro whose plugin they do
not ship, so the menu holds no `@State`: its state lives in an `ObservableObject` and in
`@AppStorage`, and the login item's status is read from `SMAppService` each time the menu is
drawn.

The app carries its own serve-markdown, so it works without one installed and runs the version
it was built with. It chooses the port itself, since serve-markdown prints the address it was
asked for rather than the one it bound, and `--port 0` would leave the port unknown. Each
instance runs in the served folder, as a run from a terminal there would, and is given the
address, the port and the editor on the command line, which stand above a configuration file,
so the menu decides them. Its output goes to a log file rather than a pipe, so nothing has to
drain it, Console opens it, and it outlives the instance for the report of an unasked exit.

## The apps are built on their own platforms by the workflow

The Windows app is built by the `windows-app` job on a Windows runner, not cross-built on
Linux. .NET builds a Windows Forms app on another platform only with `EnableWindowsTargeting`,
and Microsoft notes that such a build "is not marked as a Windows application (which means that
it will always launch a console window) and won't have an icon embedded", and that a release
should be built on Windows. The job builds `serve-markdown.exe` beside the app and publishes
the app self-contained, as a compressed single file carrying its own .NET runtime, so a user
unzips `ServeMarkdown-<version>-windows-amd64.zip` and runs it with nothing installed. Like the
executables, it is versioned from the tag, or as a dev build of the latest release on `dev`.

The macOS app is built by the `macos-app` job on an Apple silicon runner, which assembles and
signs the bundle as `./build.mjs macos` does without `MACOS_SIGN_IDENTITY`: inside-out, ad hoc,
with the hardened runtime, and uploads `ServeMarkdown-<version>-darwin-arm64.zip`. Neither app is signed
with a publicly trusted identity, since a Developer ID and an Authenticode certificate both cost
a yearly fee. Gatekeeper blocks an ad-hoc signed app that was downloaded through a browser,
until the user allows it in System Settings or removes its quarantine attribute; SmartScreen
warns about the unsigned Windows app in the same way.

The `release` and `dev-release` jobs download every artifact of the run, the executables and
the app zips alike, with `skip-decompress`: `download-artifact` unpacks an artifact served as a
zip, so without it a release would carry the files of the app zip rather than the zip.
