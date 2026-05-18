#!/usr/bin/env node
// Postinstall: download the amaru Go binary for this platform from the
// matching GitHub release and place it next to bin/amaru.js so the launcher
// can exec it. Skipped when AMARU_SKIP_DOWNLOAD=1 (useful for offline tests).

"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const https = require("https");
const { spawnSync } = require("child_process");

const pkg = require("./package.json");

const REPO = process.env.AMARU_REPO || "useamaru/amaru";
const VERSION = process.env.AMARU_VERSION || pkg.version;

function platformTarget() {
  const arch = process.arch === "x64" ? "amd64" : process.arch === "arm64" ? "arm64" : null;
  if (!arch) {
    return null;
  }
  switch (process.platform) {
    case "linux":
      return { os: "linux", arch, ext: "tar.gz", binary: "amaru" };
    case "darwin":
      return { os: "darwin", arch, ext: "tar.gz", binary: "amaru" };
    case "win32":
      return { os: "windows", arch, ext: "zip", binary: "amaru.exe" };
    default:
      return null;
  }
}

function archiveURL(version, target) {
  const base = `https://github.com/${REPO}/releases/download`;
  return `${base}/v${version}/amaru_${target.os}_${target.arch}.${target.ext}`;
}

function downloadToFile(url, dest, redirects = 0) {
  return new Promise((resolve, reject) => {
    if (redirects > 10) {
      return reject(new Error("too many redirects"));
    }
    https
      .get(url, { headers: { "user-agent": "amaru-npm-installer" } }, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302 || res.statusCode === 307 || res.statusCode === 308) {
          const next = res.headers.location;
          res.resume();
          if (!next) {
            return reject(new Error(`redirect with no Location from ${url}`));
          }
          return resolve(downloadToFile(next, dest, redirects + 1));
        }
        if (res.statusCode !== 200) {
          res.resume();
          return reject(new Error(`download ${url} returned ${res.statusCode}`));
        }
        const out = fs.createWriteStream(dest);
        res.pipe(out);
        out.on("finish", () => out.close(resolve));
        out.on("error", reject);
      })
      .on("error", reject);
  });
}

function extract(archivePath, target, destDir) {
  if (target.ext === "tar.gz") {
    const r = spawnSync("tar", ["-xzf", archivePath, "-C", destDir], { stdio: "inherit" });
    if (r.status !== 0) {
      throw new Error("tar -xzf failed (is tar installed?)");
    }
    return;
  }
  // Windows .zip — tar.exe ships with Windows 10+ and handles zip.
  const r = spawnSync("tar", ["-xf", archivePath, "-C", destDir], { stdio: "inherit" });
  if (r.status !== 0) {
    throw new Error("tar -xf failed (need tar.exe; ships with Windows 10+ or install GNU tar)");
  }
}

async function main() {
  if (process.env.AMARU_SKIP_DOWNLOAD === "1") {
    console.log("AMARU_SKIP_DOWNLOAD=1 set — skipping binary download.");
    return;
  }
  if (VERSION === "0.0.0") {
    console.log(
      "amaru: package version is 0.0.0 (unreleased). Skipping download — set AMARU_VERSION=<tag> to pull a specific build."
    );
    return;
  }

  const target = platformTarget();
  if (!target) {
    console.error(`amaru: no prebuilt binary for ${process.platform}/${process.arch}.`);
    console.error("Build from source: https://github.com/useamaru/amaru#install");
    process.exit(0); // soft-fail so npm install still succeeds
  }

  const binDir = path.join(__dirname, "bin");
  fs.mkdirSync(binDir, { recursive: true });

  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), "amaru-install-"));
  const archive = path.join(tmpRoot, `amaru.${target.ext}`);
  const url = archiveURL(VERSION, target);

  try {
    console.log(`amaru: downloading ${url}`);
    await downloadToFile(url, archive);
    extract(archive, target, tmpRoot);

    const srcBinary = path.join(tmpRoot, target.binary);
    if (!fs.existsSync(srcBinary)) {
      throw new Error(`expected ${target.binary} in archive, not found`);
    }
    const destBinary = path.join(binDir, target.binary);
    fs.copyFileSync(srcBinary, destBinary);
    fs.chmodSync(destBinary, 0o755);
    console.log(`amaru: installed ${destBinary}`);
  } catch (err) {
    console.error(`amaru: install failed: ${err.message}`);
    console.error(
      "Set AMARU_SKIP_DOWNLOAD=1 to bypass, or install manually from https://github.com/useamaru/amaru/releases"
    );
    // Soft-fail so dependents who don't actually invoke `amaru` can still install.
    process.exit(0);
  } finally {
    try {
      fs.rmSync(tmpRoot, { recursive: true, force: true });
    } catch (_) {}
  }
}

main();
