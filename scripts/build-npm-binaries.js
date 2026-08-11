#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const root = path.resolve(__dirname, "..");
const pkg = JSON.parse(fs.readFileSync(path.join(root, "package.json"), "utf8"));
const binDir = path.join(root, "bin");

const targets = [
  { os: "darwin", arch: "amd64", file: "hr-cli-darwin-amd64" },
  { os: "darwin", arch: "arm64", file: "hr-cli-darwin-arm64" },
  { os: "linux", arch: "amd64", file: "hr-cli-linux-amd64" },
  { os: "linux", arch: "arm64", file: "hr-cli-linux-arm64" },
  { os: "windows", arch: "amd64", file: "hr-cli-windows-amd64.exe" },
  { os: "windows", arch: "arm64", file: "hr-cli-windows-arm64.exe" },
];

function git(args, fallback) {
  const result = spawnSync("git", args, {
    cwd: root,
    encoding: "utf8",
    windowsHide: true,
  });
  if (result.status !== 0) return fallback;
  return result.stdout.trim() || fallback;
}

const version = process.env.HR_CLI_VERSION || pkg.version;
const commit = process.env.HR_CLI_COMMIT || git(["rev-parse", "--short=12", "HEAD"], "unknown");
const date = process.env.HR_CLI_BUILD_DATE || git(["show", "-s", "--format=%cI", "HEAD"], new Date().toISOString());

if (!version || !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`hr-cli release build: invalid package version ${JSON.stringify(version)}`);
  process.exit(1);
}

fs.mkdirSync(binDir, { recursive: true });

const ldflags = [
  "-s",
  "-w",
  `-X hr-cli/internal/build.Version=${version}`,
  `-X hr-cli/internal/build.Date=${date}`,
  `-X hr-cli/internal/build.Commit=${commit}`,
].join(" ");

function buildTarget(target) {
  const output = path.join(binDir, target.file);
  console.log(`hr-cli release build: ${target.os}/${target.arch} -> ${path.relative(root, output)}`);

  return new Promise((resolve, reject) => {
    const child = spawn(
      "go",
      ["build", "-trimpath", "-ldflags", ldflags, "-o", output, "."],
      {
        cwd: root,
        env: {
          ...process.env,
          CGO_ENABLED: "0",
          GOOS: target.os,
          GOARCH: target.arch,
        },
        stdio: "inherit",
        windowsHide: true,
      },
    );

    child.on("error", (error) => {
      reject(new Error(`could not run go for ${target.os}/${target.arch}: ${error.message}`));
    });
    child.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(`go build failed for ${target.os}/${target.arch} with exit ${code}`));
        return;
      }
      if (!fs.existsSync(output) || fs.statSync(output).size === 0) {
        reject(new Error(`missing or empty output ${output}`));
        return;
      }
      resolve();
    });
  });
}

async function main() {
  const requestedConcurrency = Number.parseInt(process.env.HR_CLI_BUILD_CONCURRENCY || "3", 10);
  const concurrency = Number.isFinite(requestedConcurrency)
    ? Math.max(1, Math.min(requestedConcurrency, targets.length))
    : 3;
  let nextTarget = 0;

  async function worker() {
    while (nextTarget < targets.length) {
      const target = targets[nextTarget];
      nextTarget += 1;
      await buildTarget(target);
    }
  }

  await Promise.all(Array.from({ length: concurrency }, () => worker()));
  console.log(`hr-cli release build: built ${targets.length} binaries for ${version} (${commit})`);
}

main().catch((error) => {
  console.error(`hr-cli release build: ${error.message}`);
  process.exitCode = 1;
});
