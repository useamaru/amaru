#!/usr/bin/env node
// Tiny launcher that execs the downloaded Go binary, forwarding argv and
// stdio. If the binary is missing, point the user at the install script.

"use strict";

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const binary = process.platform === "win32" ? "amaru.exe" : "amaru";
const binaryPath = path.join(__dirname, binary);

if (!fs.existsSync(binaryPath)) {
  console.error(`amaru: binary not found at ${binaryPath}.`);
  console.error("Run `node " + path.join(__dirname, "..", "install.js") + "` to download it,");
  console.error("or install manually from https://github.com/useamaru/amaru/releases.");
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`amaru: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
