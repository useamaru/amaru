#!/usr/bin/env node
// Best-effort cleanup of the downloaded binary on uninstall. Silently ignores
// missing files so an interrupted install never blocks uninstall.

"use strict";

const fs = require("fs");
const path = require("path");

for (const name of ["amaru", "amaru.exe"]) {
  const p = path.join(__dirname, "bin", name);
  try {
    fs.unlinkSync(p);
  } catch (_) {}
}
