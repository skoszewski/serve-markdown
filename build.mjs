#!/usr/bin/env node

// Builds, tests and checks serve-markdown, and builds the assets it embeds.
// Usage: ./build.mjs [build|test|check|clean|assets|update] [<goos>/<goarch>]
//
// The platform defaults to the host's. Naming one builds for it instead, writing
// serve-markdown-<goos>-<goarch>; it applies to the build action alone, since the tests run on
// the host. The update action moves every package web/package.json names to its latest
// release, rewriting web/package-lock.json, and builds the assets from them.

import { spawnSync } from "node:child_process";
import { readdirSync, readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const usage = "usage: ./build.mjs [build|test|check|clean|assets|update] [<goos>/<goarch>]";
const root = dirname(fileURLToPath(import.meta.url));
const [action = "build", platform = ""] = process.argv.slice(2);

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

if (!["build", "test", "check", "clean", "assets", "update"].includes(action)) {
  console.error(usage);
  process.exit(2);
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
  case "check": {
    const unformatted = run("gofmt", ["-l", "."], { capture: true }).trim();
    if (unformatted !== "") {
      console.error(`these files are not gofmt clean:\n${unformatted}`);
      process.exit(1);
    }
    run("go", ["vet", "./..."], { env });
    run("go", ["test", "./..."], { env });
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
