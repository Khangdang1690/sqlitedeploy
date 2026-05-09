#!/usr/bin/env node
// Resolver shim: locates the platform-specific sqlitedeploy binary that npm
// installed via optionalDependencies, then execs it with the user's args.
// CWD must be preserved — the Go CLI reads <cwd>/.sqlitedeploy/config.yml
// and <cwd>/.sqlitedeploy/db.sqlite, so we must not chdir here.

'use strict';

const { spawnSync } = require('node:child_process');
const path = require('node:path');

const SUPPORTED = new Set([
  'linux-x64',
  'linux-arm64',
  'darwin-x64',
  'darwin-arm64',
  'win32-x64',
  'win32-arm64',
]);

function platformKey() {
  return `${process.platform}-${process.arch}`;
}

function fail(msg) {
  process.stderr.write(`sqlitedeploy: ${msg}\n`);
  process.exit(1);
}

function resolveBinary() {
  const key = platformKey();
  if (!SUPPORTED.has(key)) {
    fail(
      `unsupported platform ${key}. ` +
        `Supported: ${[...SUPPORTED].join(', ')}. ` +
        `Download a binary manually from ` +
        `https://github.com/Khangdang1690/sqlitedeploy/releases`
    );
  }

  const exeName = process.platform === 'win32' ? 'sqlitedeploy.exe' : 'sqlitedeploy';
  // Resolve the platform package's package.json (always JSON-loadable),
  // then join to the binary. Resolving the binary directly fails because
  // Node's resolver looks for JS modules, not arbitrary files.
  const pkgManifest = `@weirdvl/${key}/package.json`;

  let manifestPath;
  try {
    manifestPath = require.resolve(pkgManifest);
  } catch (err) {
    fail(
      `platform package @weirdvl/${key} not installed. ` +
        `This usually means npm skipped optionalDependencies — try ` +
        `\`npm install --include=optional sqlitedeploy\`, or download a ` +
        `binary from https://github.com/Khangdang1690/sqlitedeploy/releases`
    );
  }
  return path.join(path.dirname(manifestPath), 'bin', exeName);
}

function main() {
  const binary = resolveBinary();
  const args = process.argv.slice(2);

  // stdio inherited so prompts (auth login) and replication logs flow through.
  // No cwd override: spawnSync defaults to process.cwd(), which is what we want.
  const result = spawnSync(binary, args, { stdio: 'inherit' });

  if (result.error) {
    if (result.error.code === 'EACCES') {
      fail(
        `binary at ${binary} is not executable. ` +
          `npm should chmod +x via the \`bin\` field — try reinstalling.`
      );
    }
    fail(`failed to spawn ${binary}: ${result.error.message}`);
  }

  // Forward signal-terminations and exit codes faithfully.
  if (result.signal) {
    process.kill(process.pid, result.signal);
    return;
  }
  process.exit(result.status ?? 0);
}

main();
