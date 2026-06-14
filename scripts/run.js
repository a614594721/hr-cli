#!/usr/bin/env node
// Thin wrapper around the prebuilt hr-cli binary installed by scripts/install.js.
// npm puts this script on PATH as `hr-cli`; we just exec the real binary.

const path = require("path");
const { spawnSync } = require("child_process");

const isWindows = process.platform === "win32";
const binary = path.join(__dirname, "..", "bin", "hr-cli" + (isWindows ? ".exe" : ""));

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
process.exit(result.status === null ? 1 : result.status);
