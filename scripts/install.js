#!/usr/bin/env node
// Postinstall script for @wubang9527/hr-cli.
// Bundled binaries for darwin/linux/windows × amd64/arm64 ship inside the
// npm tarball under bin/. This script picks the one that matches the
// current platform and copies it to bin/hr-cli (or bin/hr-cli.exe on
// Windows). No network access required.

const fs = require("fs");
const path = require("path");

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!platform || !arch) {
  console.error(`hr-cli: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const binDir = path.join(__dirname, "..", "bin");
const sourceName = `hr-cli-${platform}-${arch}` + (isWindows ? ".exe" : "");
const source = path.join(binDir, sourceName);
const dest = path.join(binDir, "hr-cli" + (isWindows ? ".exe" : ""));

if (!fs.existsSync(source)) {
  console.error(`hr-cli: bundled binary not found for ${platform}/${arch} at ${source}`);
  process.exit(1);
}

try {
  fs.copyFileSync(source, dest);
  if (!isWindows) fs.chmodSync(dest, 0o755);
  console.log(`hr-cli: installed ${platform}/${arch} binary to ${dest}`);
} catch (err) {
  console.error(`hr-cli: install failed: ${err.message}`);
  process.exit(1);
}
