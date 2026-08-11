#!/usr/bin/env node

const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

const root = path.resolve(__dirname, "..");
const pkg = JSON.parse(fs.readFileSync(path.join(root, "package.json"), "utf8"));
const expectedBinaries = [
  "hr-cli-darwin-amd64",
  "hr-cli-darwin-arm64",
  "hr-cli-linux-amd64",
  "hr-cli-linux-arm64",
  "hr-cli-windows-amd64.exe",
  "hr-cli-windows-arm64.exe",
];

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || root,
    encoding: "utf8",
    windowsHide: true,
    ...options,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const output = [result.stdout, result.stderr].filter(Boolean).join("\n").trim();
    throw new Error(`${command} ${args.join(" ")} failed with exit ${result.status}${output ? `\n${output}` : ""}`);
  }
  return result;
}

function runNpm(args, options = {}) {
  if (process.env.npm_execpath) {
    return run(process.execPath, [process.env.npm_execpath, ...args], options);
  }
  return run("npm", args, {
    ...options,
    shell: process.platform === "win32",
  });
}

let tarball;
let tempDir;

try {
  const packed = runNpm(["pack", "--silent"]);
  const packedLines = packed.stdout.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  const tarballName = [...packedLines].reverse().find((line) => line.endsWith(".tgz"));
  if (!tarballName) throw new Error(`npm pack did not return a tarball name: ${packed.stdout}`);

  tarball = path.resolve(root, tarballName);
  if (!tarball.startsWith(`${root}${path.sep}`) || !fs.existsSync(tarball)) {
    throw new Error(`unsafe or missing npm tarball path: ${tarball}`);
  }

  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "hr-cli-release-test-"));
  runNpm(["install", "--prefix", tempDir, tarball, "--no-audit", "--no-fund"]);

  const installedRoot = path.join(tempDir, "node_modules", "@wubang9527", "hr-cli");
  const installedPackage = JSON.parse(fs.readFileSync(path.join(installedRoot, "package.json"), "utf8"));
  if (installedPackage.version !== pkg.version) {
    throw new Error(`installed package version ${installedPackage.version} does not match ${pkg.version}`);
  }

  for (const binary of expectedBinaries) {
    const binaryPath = path.join(installedRoot, "bin", binary);
    if (!fs.existsSync(binaryPath) || fs.statSync(binaryPath).size === 0) {
      throw new Error(`packaged binary is missing or empty: ${binary}`);
    }
  }

  const cliWrapper = path.join(installedRoot, "scripts", "run.js");
  const versionResult = run(process.execPath, [cliWrapper, "--version"], { cwd: tempDir });
  const reportedVersion = versionResult.stdout.trim();
  const expectedVersion = `hr version ${pkg.version}`;
  if (reportedVersion !== expectedVersion) {
    throw new Error(`CLI reported ${JSON.stringify(reportedVersion)}; expected ${JSON.stringify(expectedVersion)}`);
  }

  console.log(`hr-cli release test: ${pkg.name}@${pkg.version} installed cleanly`);
  console.log(`hr-cli release test: verified ${expectedBinaries.length} bundled binaries`);
  console.log(`hr-cli release test: ${reportedVersion}`);
} catch (error) {
  console.error(`hr-cli release test: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (tempDir && tempDir.startsWith(`${path.resolve(os.tmpdir())}${path.sep}`)) {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
  if (tarball && tarball.startsWith(`${root}${path.sep}`) && tarball.endsWith(".tgz")) {
    fs.rmSync(tarball, { force: true });
  }
}
