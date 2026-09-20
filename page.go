package main

import (
	"fmt"
	"html"
)

// assetURLs names the browser-side libraries the page shell loads.
type assetURLs struct {
	markdownCSS      string
	highlightCSSLite string
	highlightCSSDark string
	markedJS         string
	domPurifyJS      string
	highlightJS      string
}

// cdnAssets loads the libraries from their public CDNs, as --online asks for.
var cdnAssets = assetURLs{
	markdownCSS:      "https://cdn.jsdelivr.net/npm/github-markdown-css@5/github-markdown.min.css",
	highlightCSSLite: "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css",
	highlightCSSDark: "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css",
	markedJS:         "https://cdn.jsdelivr.net/npm/marked@15/marked.min.js",
	domPurifyJS:      "https://cdn.jsdelivr.net/npm/dompurify@3/dist/purify.min.js",
	highlightJS:      "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js",
}

// embeddedAssets loads the same libraries from the copies built into the binary.
var embeddedAssets = assetURLs{
	markdownCSS:      assetRoute + "github-markdown.min.css",
	highlightCSSLite: assetRoute + "github.min.css",
	highlightCSSDark: assetRoute + "github-dark.min.css",
	markedJS:         assetRoute + "marked.min.js",
	domPurifyJS:      assetRoute + "purify.min.js",
	highlightJS:      assetRoute + "highlight.min.js",
}

const pageScript = `<article id="content" class="markdown-body">Loading...</article>
<script>
  let lastMtime = null;
  function render(text, css) {
    const target = document.getElementById("content");
    document.getElementById("document-css").textContent = css || "";
    if (window.marked && window.DOMPurify) {
      target.innerHTML = DOMPurify.sanitize(marked.parse(text, {gfm: true}));
      if (window.hljs) {
        target.querySelectorAll("pre code").forEach((block) => hljs.highlightElement(block));
      }
    } else {
      const pre = document.createElement("pre");
      pre.textContent = text;
      target.innerHTML = "";
      target.appendChild(pre);
    }
  }
  async function poll() {
    let data;
    try {
      data = await (await fetch("/content%s")).json();
    } catch (error) {
      return;
    }
    if (data.mtime !== null && data.mtime !== lastMtime) {
      lastMtime = data.mtime;
      render(data.text, data.css);
    } else if (data.mtime === null) {
      render("(file not found: " + data.error + ")", "");
    }
  }
  poll();
  setInterval(poll, %d);
</script>`

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<link rel="stylesheet" href="%s">
<link rel="stylesheet" href="%s" media="(prefers-color-scheme: light)">
<link rel="stylesheet" href="%s" media="(prefers-color-scheme: dark)">
<script src="%s"></script>
<script src="%s"></script>
<script src="%s"></script>
<style>
  body { margin: 0; background-color: #ffffff; }
  @media (prefers-color-scheme: dark) {
    body { background-color: #0d1117; }
  }
  .markdown-body { box-sizing: border-box; max-width: 980px; margin: 0 auto; padding: 45px; }
  .markdown-body pre { white-space: pre; overflow-x: auto; }
</style>
<style id="document-css"></style>
</head>
<body>
%s
</body>
</html>
`

// renderPage builds the HTML page shell that polls /content and renders it as Markdown.
//
// contentQuery is the query string, including its leading '?', appended to the /content
// request, and watchIntervalMS the milliseconds between polls.
func renderPage(title, contentQuery string, watchIntervalMS int, assets assetURLs) []byte {
	body := fmt.Sprintf(pageScript, contentQuery, watchIntervalMS)
	page := fmt.Sprintf(pageTemplate,
		html.EscapeString(title),
		assets.markdownCSS, assets.highlightCSSLite, assets.highlightCSSDark,
		assets.markedJS, assets.domPurifyJS, assets.highlightJS,
		body,
	)
	return []byte(page)
}
