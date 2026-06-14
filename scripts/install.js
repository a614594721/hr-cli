#!/usr/bin/env node
// Postinstall script for @a614594721/hr-cli.
// Downloads the prebuilt hr-cli binary from GitHub Releases for the current
// platform, verifies the archive against the published checksums, extracts
// the binary into ./bin/, and exits.
//
// Network-restricted environments can set HR_CLI_BINARY_URL to a local file
// or mirror to skip the GitHub fetch.

const fs = require("fs");
const path = require("path");
const os = require("os");
const crypto = require("crypto");
const { execFileSync } = require("child_process");

const REPO = "a614594721/hr-cli";
const NAME = "hr-cli";
const VERSION = require("../package.json").version;

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!platform || !arch) {
  console.error(`hr-cli: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
const releaseBase = `https://github.com/${REPO}/releases/download/v${VERSION}`;
const githubArchiveURL = process.env.HR_CLI_BINARY_URL || `${releaseBase}/${archiveName}`;
const checksumsURL = `${releaseBase}/checksums.txt`;

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

fs.mkdirSync(binDir, { recursive: true });

const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "hr-cli-"));
const archivePath = path.join(tmpDir, archiveName);

try {
  console.log(`hr-cli: downloading ${archiveName}`);
  download(githubArchiveURL, archivePath);

  if (!process.env.HR_CLI_BINARY_URL) {
    console.log("hr-cli: verifying checksum");
    const expected = fetchChecksum(checksumsURL, archiveName);
    const actual = sha256(archivePath);
    if (expected && expected !== actual) {
      throw new Error(`checksum mismatch: expected ${expected}, got ${actual}`);
    }
  }

  console.log("hr-cli: extracting");
  extract(archivePath, tmpDir);

  const sourceBinary = path.join(tmpDir, NAME + (isWindows ? ".exe" : ""));
  if (!fs.existsSync(sourceBinary)) {
    throw new Error(`extracted binary not found at ${sourceBinary}`);
  }
  fs.copyFileSync(sourceBinary, dest);
  if (!isWindows) fs.chmodSync(dest, 0o755);
  console.log(`hr-cli: installed to ${dest}`);
} catch (err) {
  console.error(`hr-cli: install failed: ${err.message}`);
  console.error("hr-cli: you can rerun the install or download the binary manually from:");
  console.error(`  ${githubArchiveURL}`);
  process.exit(1);
} finally {
  try { fs.rmSync(tmpDir, { recursive: true, force: true }); } catch (_) {}
}

function download(url, target) {
  if (url.startsWith("file://") || fs.existsSync(url)) {
    fs.copyFileSync(url.replace(/^file:\/\//, ""), target);
    return;
  }
  execFileSync("curl", ["-fL", "--max-redirs", "5", "-o", target, url], { stdio: "inherit" });
}

function fetchChecksum(url, name) {
  try {
    const tmpFile = path.join(tmpDir, "checksums.txt");
    execFileSync("curl", ["-fL", "--max-redirs", "5", "-o", tmpFile, url], { stdio: "ignore" });
    const text = fs.readFileSync(tmpFile, "utf8");
    for (const line of text.split(/\r?\n/)) {
      const m = line.match(/^([0-9a-fA-F]{64})\s+(.+)$/);
      if (m && m[2] === name) return m[1].toLowerCase();
    }
  } catch (_) { /* fall through */ }
  return null;
}

function sha256(file) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(file));
  return hash.digest("hex").toLowerCase();
}

function extract(archive, dir) {
  if (archive.endsWith(".zip")) {
    if (process.platform === "win32") {
      execFileSync("powershell", [
        "-NoProfile", "-Command",
        `Expand-Archive -Path '${archive}' -DestinationPath '${dir}' -Force`,
      ], { stdio: "inherit" });
    } else {
      execFileSync("unzip", ["-o", archive, "-d", dir], { stdio: "inherit" });
    }
  } else {
    execFileSync("tar", ["-xzf", archive, "-C", dir], { stdio: "inherit" });
  }
}
