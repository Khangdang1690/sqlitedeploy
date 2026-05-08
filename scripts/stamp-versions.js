#!/usr/bin/env node
// Rewrite version fields in all npm package manifests.
//   node scripts/stamp-versions.js 0.1.0
// Affects:
//   packaging/npm/sqlitedeploy/package.json   (main package + optionalDependencies)
//   packaging/npm/platforms/<plat>/package.json (each platform package)

'use strict';

const fs = require('node:fs');
const path = require('node:path');

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+/.test(version)) {
  console.error('usage: stamp-versions.js <semver>');
  process.exit(1);
}

const repoRoot = path.resolve(__dirname, '..');
const platformsDir = path.join(repoRoot, 'packaging', 'npm', 'platforms');
const mainPkgPath = path.join(repoRoot, 'packaging', 'npm', 'sqlitedeploy', 'package.json');

function rewrite(file, mutate) {
  const json = JSON.parse(fs.readFileSync(file, 'utf8'));
  mutate(json);
  fs.writeFileSync(file, JSON.stringify(json, null, 2) + '\n');
  console.log(`stamped ${path.relative(repoRoot, file)} -> ${version}`);
}

// Each platform package gets the version directly.
for (const dir of fs.readdirSync(platformsDir)) {
  const pkg = path.join(platformsDir, dir, 'package.json');
  if (!fs.existsSync(pkg)) continue;
  rewrite(pkg, (json) => {
    json.version = version;
  });
}

// Main package: bump its own version AND every entry in optionalDependencies
// to the same version, so the resolver can require them at exactly that pin.
rewrite(mainPkgPath, (json) => {
  json.version = version;
  if (json.optionalDependencies) {
    for (const dep of Object.keys(json.optionalDependencies)) {
      json.optionalDependencies[dep] = version;
    }
  }
});
