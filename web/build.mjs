// Builds the browser-side assets the server embeds into ../assets/build: the page script
// bundled with the libraries it imports, the stylesheets, mermaid, and the licences of all of
// them.

import { build } from "esbuild";
import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";

const output = "../assets/build";
const modules = "node_modules";

// The packages whose code ends up in the output, by bundling or copying.
const packages = ["marked", "dompurify", "highlight.js", "github-markdown-css", "mermaid"];

await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });

await build({
  entryPoints: ["src/page.js"],
  outfile: join(output, "page.js"),
  bundle: true,
  minify: true,
  format: "iife",
  legalComments: "eof",
});

await build({
  entryPoints: [
    { in: join(modules, "github-markdown-css/github-markdown.css"), out: "github-markdown" },
    { in: join(modules, "highlight.js/styles/github.css"), out: "highlight-light" },
    { in: join(modules, "highlight.js/styles/github-dark.css"), out: "highlight-dark" },
  ],
  outdir: output,
  minify: true,
  legalComments: "eof",
});

await copyFile(join(modules, "mermaid/dist/mermaid.min.js"), join(output, "mermaid.min.js"));

const rows = [];
for (const name of packages) {
  const manifest = JSON.parse(await readFile(join(modules, name, "package.json"), "utf8"));
  rows.push(`| ${manifest.name} | ${manifest.version} | ${manifest.license} |`);
}
await writeFile(join(output, "LICENSES.md"), [
  "# Bundled libraries",
  "",
  "Built by `web/build.mjs` from the packages pinned in `web/package-lock.json`; each is",
  "redistributed under its own licence.",
  "",
  "| Package | Version | Licence |",
  "|---|---|---|",
  ...rows,
  "",
].join("\n"));
