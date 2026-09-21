# Architectural Decision Records

## The page shell lives in embedded files rather than in Go literals

The shell was two Go string constants assembled with `fmt.Sprintf`, whose positional
arguments had to be kept in step by hand. The outline and the diagram support roughly tripled
the script.

The shell is now `assets/page/page.html`, `page.css` and `page.js`, embedded with `go:embed`.
The HTML is rendered with `html/template` and the settings the script needs are written into
the page as one `pageConfig` object, which `html/template` escapes for a script context. The
styling and the script are served from the binary under `/_/page/`, whatever `--online` says,
since they are the server's own rather than a third party's, and the browser caches them
between page loads.

## The outline is numbered in CSS, not in JavaScript

The script only nests the lists, following the heading levels; each `--outline` style is a
class on the outline, and the numbers come from CSS counters. `numbered-hierarchical` is
`counters(section, ".")`, which writes `1.`, `1.1.`, `1.1.1.` without the script knowing the
numbering at all, so a style is added by writing a rule.

## Mermaid is the UMD bundle, loaded on first use

Mermaid publishes an ES module that pulls its diagram types from a `chunks` directory of some
twenty megabytes, and a single self-contained `dist/mermaid.min.js` that sets
`globalThis.mermaid`. The single file is vendored, so one more file keeps the server free of
a network of its own, at 5.5 MB of the binary.

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

## A folder's document is looked for the way its source writes it

A local directory is read as its `index.md` before its `README.md`, and an Azure Repos folder
the other way round, since that is the name each is usually given where it stands. The order
is the source's, not a setting, so a folder resolves to the same document however it is
reached.

`--index-only` stops the search at those two names: a folder is the document it holds or a
page saying none was found, and the listing of whatever else stands there is left out.

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

The command line stands above the file: a file belongs to a directory and is read by everyone
who serves it, while a flag belongs to one run. Which flags were written is read from the flag
package itself rather than by comparing against defaults, so writing a flag's own default
still counts as having written it.

Unknown keys are refused rather than passed over. A file that quietly ignores `outlyne: true`
teaches the reader that the setting does not work, and says nothing about why.

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

A repository holding a `README.md` and nothing else would list that one document, which is the
document the reader is already looking at. The list is then the project's repositories, the
one on the page marked, so the sidebar always offers somewhere to go. The link above them
leads to the project as it always does, under a label naming what the list now holds.

That swap belongs to the repository's root alone. A folder within a repository keeps its own
list however short it is, since the folder above it is a step the reader will want, and the
link above the list is the one that takes it: **Up** below the root, **Browse to
repositories** at it.

Telling that case apart takes more than the entry's mark: a repository's own route addresses
the document inside it without naming it, so nothing in the list is marked. An only entry
named `README.md` or `index.md` is therefore read as the document on the page, while an only
entry under another name is kept - the page did not resolve to it, and that entry is the only
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
