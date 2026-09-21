# Vendored libraries

These files are the browser-side libraries the page shell loads. They are served from inside
the binary, and are unused when `--online` is given: the page then loads the same builds from
their CDNs. Each is redistributed under its own licence, reproduced upstream. `./vendor.sh`
and `./vendor.ps1` download them at the versions below.

| File | Library | Version | Licence |
|---|---|---|---|
| `marked.min.js` | [marked](https://github.com/markedjs/marked) | 15 | MIT |
| `purify.min.js` | [DOMPurify](https://github.com/cure53/DOMPurify) | 3 | Apache-2.0 OR MPL-2.0 |
| `highlight.min.js` | [highlight.js](https://github.com/highlightjs/highlight.js) | 11.9.0 | BSD-3-Clause |
| `github.min.css` | highlight.js GitHub light theme | 11.9.0 | BSD-3-Clause |
| `github-dark.min.css` | highlight.js GitHub dark theme | 11.9.0 | BSD-3-Clause |
| `github-markdown.min.css` | [github-markdown-css](https://github.com/sindresorhus/github-markdown-css) | 5 | MIT |
| `mermaid.min.js` | [mermaid](https://github.com/mermaid-js/mermaid) | 12.0.0 | MIT |
