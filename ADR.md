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

## A route below an ado:// source resolves beside the document

A local file source makes the file's directory the server's root, so a route below it
addresses a path beside the file. An `ado://` source resolved the same route against the
repository root instead, which left a picture beside the document unreachable. Both now
resolve beside the document.
