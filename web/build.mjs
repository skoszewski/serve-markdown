// Builds the browser-side assets the server embeds: the page script, the stylesheets and mermaid
// into ../render/assets/build, the editor into ../assets/build, each bundled with the libraries it
// imports, and in each directory the licences of the packages it holds.

import { build } from "esbuild";
import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";

const pageOutput = "../render/assets/build";
const editorOutput = "../assets/build";
const modules = "node_modules";

// The packages whose code ends up in each output, by bundling or copying.
const pagePackages = ["marked", "dompurify", "highlight.js", "github-markdown-css", "mermaid"];
const editorPackages = [
  "react",
  "react-dom",
  "@mdxeditor/editor",
  "@headlessui/react",
  "@codemirror/language",
  "@lezer/highlight",
];

for (const output of [pageOutput, editorOutput]) {
  await rm(output, { recursive: true, force: true });
  await mkdir(output, { recursive: true });
}

await build({
  entryPoints: ["src/page.js"],
  outfile: join(pageOutput, "page.js"),
  bundle: true,
  minify: true,
  format: "iife",
  legalComments: "eof",
});

// The editor application with React and the libraries it uses, its stylesheets bundled into editor.css.
await build({
  entryPoints: ["src/editor/main.tsx"],
  outfile: join(editorOutput, "editor.js"),
  bundle: true,
  minify: true,
  format: "iife",
  jsx: "automatic",
  define: { "process.env.NODE_ENV": '"production"' },
  legalComments: "eof",
});

await build({
  entryPoints: [
    { in: join(modules, "github-markdown-css/github-markdown-light.css"), out: "github-markdown-light" },
    { in: join(modules, "github-markdown-css/github-markdown-dark.css"), out: "github-markdown-dark" },
    { in: join(modules, "highlight.js/styles/github.css"), out: "highlight-light" },
    { in: join(modules, "highlight.js/styles/github-dark.css"), out: "highlight-dark" },
  ],
  outdir: pageOutput,
  minify: true,
  legalComments: "eof",
});

await copyFile(join(modules, "mermaid/dist/mermaid.min.js"), join(pageOutput, "mermaid.min.js"));

for (const [output, packages] of [[pageOutput, pagePackages], [editorOutput, editorPackages]]) {
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
}
