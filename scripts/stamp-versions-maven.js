#!/usr/bin/env node
// Rewrite the project version in every pom.xml under packaging/maven/.
//   node scripts/stamp-versions-maven.js 0.1.0
// Affects:
//   packaging/maven/pom.xml             (parent project version)
//   packaging/maven/launcher/pom.xml    (parent reference)
//   packaging/maven/platforms/*/pom.xml (parent reference)
//
// Each pom.xml has its project-or-parent <version> tag as the *first*
// <version>...</version> occurrence, before any plugin/dependency versions.
// Replacing only the first match avoids touching plugin versions like
// `<version>3.4.2</version>` for maven-jar-plugin.

'use strict';

const fs = require('node:fs');
const path = require('node:path');

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+/.test(version)) {
  console.error('usage: stamp-versions-maven.js <semver>');
  process.exit(1);
}

const repoRoot = path.resolve(__dirname, '..');
const mavenDir = path.join(repoRoot, 'packaging', 'maven');

function findPoms(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (entry.name === 'target' || entry.name === 'node_modules') continue;
      out.push(...findPoms(path.join(dir, entry.name)));
    } else if (entry.name === 'pom.xml') {
      out.push(path.join(dir, entry.name));
    }
  }
  return out;
}

function stamp(file, version) {
  const before = fs.readFileSync(file, 'utf8');
  const after = before.replace(/<version>[^<]+<\/version>/, `<version>${version}</version>`);
  if (after === before) {
    throw new Error(`no <version> tag found in ${file}`);
  }
  fs.writeFileSync(file, after);
  console.log(`stamped ${path.relative(repoRoot, file)} -> ${version}`);
}

for (const pom of findPoms(mavenDir)) {
  stamp(pom, version);
}
