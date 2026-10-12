#!/usr/bin/env node

// Builds, tests, checks and releases serve-markdown, and builds the assets it embeds.
// Usage: ./build.mjs [build|test|check|clean|assets|update] [<goos>/<goarch>]
//        ./build.mjs release vX.Y.Z
//
// The platform defaults to the host's. Naming one builds for it instead, writing
// serve-markdown-<goos>-<goarch>; it applies to the build action alone, since the tests run on
// the host. The update action moves every package web/package.json names to its latest
// release, rewriting web/package-lock.json, and builds the assets from them. The release
// action checks dev, tags it with notes edited in $EDITOR, prefilled with the commit subjects
// since the previous tag, fast-forwards main to it, and pushes dev, main and the tag together.

import { spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const usage = "usage: ./build.mjs [build|test|check|clean|assets|update] [<goos>/<goarch>]\n" +
  "       ./build.mjs release vX.Y.Z";
const root = dirname(fileURLToPath(import.meta.url));
const [action = "build", argument = ""] = process.argv.slice(2);
const platform = action === "release" ? "" : argument;
const tag = action === "release" ? argument : "";

/**
 * Runs a command in the given directory and returns what it wrote to stdout; the command's
 * failure ends the build with its exit status.
 */
function run(command, args, { cwd = root, env = process.env, capture = false } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    stdio: capture ? ["inherit", "pipe", "inherit"] : "inherit",
    encoding: "utf8",
    shell: process.platform === "win32" && command === "npm",
  });
  if (result.error) {
    console.error(`${command}: ${result.error.message}`);
    process.exit(1);
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
  return result.stdout;
}

let binary = "serve-markdown";
const env = { ...process.env, CGO_ENABLED: "0" };

if (platform !== "") {
  const [goos, goarch, ...rest] = platform.split("/");
  if (!goos || !goarch || rest.length > 0) {
    console.error("platform must be given as <goos>/<goarch>, e.g. linux/amd64");
    process.exit(2);
  }
  env.GOOS = goos;
  env.GOARCH = goarch;
  binary = `serve-markdown-${goos}-${goarch}${goos === "windows" ? ".exe" : ""}`;
}

if (!["build", "test", "check", "clean", "assets", "update", "release"].includes(action)) {
  console.error(usage);
  process.exit(2);
}

// A release is made from a clean dev, for a semver tag with a leading v, as the workflow
// requires.
if (action === "release") {
  const number = "(0|[1-9][0-9]*)";
  const identifier = "(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)";
  const semver = new RegExp(`^v${number}\\.${number}\\.${number}(-${identifier}(\\.${identifier})*)?$`);
  if (!semver.test(tag)) {
    console.error(`${usage}\nthe version must be a semver tag with a leading v, e.g. v2.4.0`);
    process.exit(2);
  }
  if (run("git", ["branch", "--show-current"], { capture: true }).trim() !== "dev") {
    console.error("a release is made from dev");
    process.exit(2);
  }
  if (run("git", ["status", "--porcelain"], { capture: true }).trim() !== "") {
    console.error("the working tree is not clean");
    process.exit(2);
  }
}

// The latest release of each package is installed even across a major version, which the
// ranges in web/package.json would not allow.
if (action === "update") {
  const manifest = JSON.parse(readFileSync(`${root}/web/package.json`, "utf8"));
  const latest = Object.keys(manifest.devDependencies).map((name) => `${name}@latest`);
  run("npm", ["install", "--save-dev", ...latest], { cwd: `${root}/web` });
}

// The Go code embeds the built assets, so every action compiling it builds them first.
if (action !== "clean") {
  run("npm", ["ci"], { cwd: `${root}/web` });
  run("npm", ["run", "build"], { cwd: `${root}/web` });
}

switch (action) {
  case "build":
    run("go", ["build", "-o", binary, "."], { env });
    break;
  case "test":
    run("go", ["test", "./..."], { env });
    break;
  case "check":
  case "release": {
    const unformatted = run("gofmt", ["-l", "."], { capture: true }).trim();
    if (unformatted !== "") {
      console.error(`these files are not gofmt clean:\n${unformatted}`);
      process.exit(1);
    }
    run("go", ["vet", "./..."], { env });
    run("go", ["test", "./..."], { env });
    if (action === "release") {
      // The notes are edited with ';' as git's comment character, so Markdown headings stay.
      const previous = run("git", ["describe", "--tags", "--abbrev=0", "--match", "v[0-9]*", "dev"], { capture: true }).trim();
      const subjects = run("git", ["log", `${previous}..dev`, "--format=- %s"], { capture: true });
      const notesDir = mkdtempSync(join(tmpdir(), "serve-markdown-"));
      const notes = join(notesDir, "notes.md");
      writeFileSync(notes, `## Changed\n\n${subjects}`);
      const tagged = spawnSync("git", ["-c", "core.commentChar=;", "tag", "-a", tag, "-e", "-F", notes, "dev"], { cwd: root, stdio: "inherit" });
      rmSync(notesDir, { recursive: true, force: true });
      if (tagged.status !== 0) {
        process.exit(tagged.status ?? 1);
      }
      run("git", ["fetch", ".", "dev:main"]);
      run("git", ["push", "--atomic", "origin", "dev", "main", tag]);
    }
    break;
  }
  case "clean":
    for (const name of readdirSync(root)) {
      if (name === "serve-markdown" || name === "serve-markdown.exe" || name.startsWith("serve-markdown-")) {
        rmSync(`${root}/${name}`, { force: true });
      }
    }
    break;
}
