#!/usr/bin/env node

// Builds, tests, checks and releases serve-markdown, and builds the assets it embeds.
// Usage: ./build.mjs [build|test|check|clean|assets|update|macos] [<goos>/<goarch>]
//        ./build.mjs windows [self-contained]
//        ./build.mjs release vX.Y.Z
//
// The platform defaults to the host's. Naming one builds for it instead, writing
// serve-markdown-<goos>-<goarch>; it applies to the build action alone. The update action moves
// every package web/package.json names to its latest release, rewriting web/package-lock.json,
// and builds the assets from them. The macos action, on macOS alone, writes the menu bar app
// dist/ServeMarkdown.app with serve-markdown inside it, signed by $MACOS_SIGN_IDENTITY, ad-hoc
// when it is not set. The windows action writes the notification-area app for Windows x64 and
// serve-markdown.exe into dist/ServeMarkdown-windows; the app runs on the installed .NET
// Desktop Runtime, or carries its own with self-contained. The release action checks dev, tags
// it with notes edited in $EDITOR, prefilled with the commit subjects since the previous tag,
// fast-forwards main to it, and pushes dev, main and the tag together.

import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const usage = "usage: ./build.mjs [build|test|check|clean|assets|update|macos] [<goos>/<goarch>]\n" +
  "       ./build.mjs windows [self-contained]\n" +
  "       ./build.mjs release vX.Y.Z";
const root = dirname(fileURLToPath(import.meta.url));
const [action = "build", argument = ""] = process.argv.slice(2);
const platform = ["release", "windows"].includes(action) ? "" : argument;
const selfContained = action === "windows" && argument === "self-contained";
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

if (!["build", "test", "check", "clean", "assets", "update", "macos", "windows", "release"].includes(action)) {
  console.error(usage);
  process.exit(2);
}

if (action === "windows" && !["", "self-contained"].includes(argument)) {
  console.error(usage);
  process.exit(2);
}

if (action === "macos" && (process.platform !== "darwin" || platform !== "")) {
  console.error("the macos action builds on macOS, for the host");
  process.exit(2);
}

// A release is made from a clean dev, for a semver tag with a leading v.
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

// The latest release of each package is installed, across major versions.
if (action === "update") {
  const manifest = JSON.parse(readFileSync(`${root}/web/package.json`, "utf8"));
  const latest = Object.keys(manifest.devDependencies).map((name) => `${name}@latest`);
  run("npm", ["install", "--save-dev", ...latest], { cwd: `${root}/web` });
}

// Every action but clean builds the assets first.
if (action !== "clean") {
  run("npm", ["ci"], { cwd: `${root}/web` });
  run("npm", ["run", "build"], { cwd: `${root}/web` });
}

// The apps report the latest release without its prerelease suffix; their serve-markdown names
// itself a dev build of that release.
const latest = ["macos", "windows"].includes(action)
  ? run("git", ["describe", "--tags", "--abbrev=0", "--match", "v[0-9]*"], { capture: true }).trim()
  : "";
const devVersion = `${latest}-dev-${new Date().toISOString().slice(0, 10).replaceAll("-", "")}`;
const shortVersion = latest.replace(/^v/, "").replace(/-.*$/, "");

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
      // The notes are edited with ';' as git's comment character.
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
  case "macos": {
    const app = `${root}/dist/ServeMarkdown.app`;
    const contents = `${app}/Contents`;
    rmSync(app, { recursive: true, force: true });
    mkdirSync(`${contents}/MacOS`, { recursive: true });

    run("go", ["build", "-trimpath", "-ldflags", `-X main.buildVersion=${devVersion}`,
      "-o", `${contents}/MacOS/serve-markdown`, "."], { env });

    const swiftEnv = { ...process.env, MACOSX_DEPLOYMENT_TARGET: "13.0" };
    const swiftArgs = ["build", "-c", "release", "--package-path", `${root}/macos`];
    run("swift", swiftArgs, { env: swiftEnv });
    const binPath = run("swift", [...swiftArgs, "--show-bin-path"], { env: swiftEnv, capture: true }).trim();
    copyFileSync(`${binPath}/ServeMarkdown`, `${contents}/MacOS/ServeMarkdown`);

    const infoPlist = readFileSync(`${root}/macos/Info.plist.in`, "utf8").replaceAll("@SHORT_VERSION@", shortVersion);
    writeFileSync(`${contents}/Info.plist`, infoPlist);
    run("plutil", ["-lint", `${contents}/Info.plist`]);
    writeFileSync(`${contents}/PkgInfo`, "APPL????");

    const identity = process.env.MACOS_SIGN_IDENTITY || "-";
    const timestamp = identity === "-" ? "--timestamp=none" : "--timestamp";
    for (const target of [`${contents}/MacOS/serve-markdown`, app]) {
      run("codesign", ["--force", "--sign", identity, timestamp, "--options", "runtime",
        "--entitlements", `${root}/macos/entitlements.plist`, target]);
    }
    run("codesign", ["--verify", "--strict", "--deep", "--verbose=2", app]);
    break;
  }
  case "windows": {
    const out = `${root}/dist/ServeMarkdown-windows`;
    rmSync(out, { recursive: true, force: true });
    run("go", ["build", "-trimpath", "-ldflags", `-X main.buildVersion=${devVersion}`,
      "-o", `${out}/serve-markdown.exe`, "."], { env: { ...env, GOOS: "windows", GOARCH: "amd64" } });
    run("dotnet", ["publish", `${root}/windows/ServeMarkdown.csproj`, "-c", "Release", "-o", out,
      `-p:Version=${shortVersion}`, "--self-contained", String(selfContained),
      ...(selfContained ? ["-p:EnableCompressionInSingleFile=true"] : [])]);
    break;
  }
  case "clean":
    for (const name of readdirSync(root)) {
      if (name === "serve-markdown" || name === "serve-markdown.exe" || name.startsWith("serve-markdown-")) {
        rmSync(`${root}/${name}`, { force: true });
      }
    }
    rmSync(`${root}/dist/ServeMarkdown.app`, { recursive: true, force: true });
    rmSync(`${root}/macos/.build`, { recursive: true, force: true });
    rmSync(`${root}/dist/ServeMarkdown-windows`, { recursive: true, force: true });
    rmSync(`${root}/windows/bin`, { recursive: true, force: true });
    rmSync(`${root}/windows/obj`, { recursive: true, force: true });
    break;
}
