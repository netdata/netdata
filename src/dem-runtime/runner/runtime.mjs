// SPDX-License-Identifier: GPL-3.0-or-later
import fs from 'node:fs/promises';
import {constants} from 'node:fs';
import path from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const readJSON = async file => JSON.parse(await fs.readFile(file, 'utf8'));

// Locations are release-owned. Monitor requests cannot replace any component.
export async function runtimePaths() {
  const metadata = await readJSON(path.join(root, 'runtime.json'));
  if (metadata.contract !== 1 || typeof metadata.version !== 'string' ||
      !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(metadata.version)) {
    throw new Error('invalid runtime execution identity');
  }
  const paths = {};
  for (const key of ['dependencies', 'browser']) {
    const value = metadata.paths?.[key];
    if (typeof value !== 'string' || !value || path.isAbsolute(value) ||
        value.split('/').some(part => !part || part === '.' || part === '..') || value.includes('\\')) {
      throw new Error('invalid runtime component path: ' + key);
    }
    paths[key] = path.join(root, value);
  }
  return {...paths, modules: path.join(paths.dependencies, 'node_modules'), metadata};
}

export async function checkRuntime() {
  const runtime = await runtimePaths();
  const pins = await readJSON(path.join(root, 'manifest.json'));
  if (pins.protocol_version !== runtime.metadata.contract ||
      Number(process.versions.node.split('.')[0]) < pins.node_major_min || process.versions.node !== pins.node_version) {
    throw new Error('runtime Node or execution contract mismatch');
  }
  for (const [name, version] of Object.entries(pins.packages)) {
    const installed = await readJSON(path.join(runtime.modules, name, 'package.json'));
    if (installed.version !== version) throw new Error('runtime package version mismatch: ' + name);
  }
  // Check the actual private import surfaces consumed by both execution modes.
  for (const module of ['@playwright/test/index.mjs', 'playwright/index.mjs',
    'lighthouse/core/index.js', 'lighthouse/core/config/lr-desktop-config.js',
    'chrome-launcher/dist/index.js']) {
    await import(pathToFileURL(path.join(runtime.modules, module)));
  }
  await fs.access(runtime.browser, constants.X_OK);
  const browser = execFileSync(runtime.browser, ['--version'], {
    encoding: 'utf8', timeout: 5000, maxBuffer: 4096, stdio: ['ignore', 'pipe', 'pipe'],
    env: {PATH: path.dirname(process.execPath) + ':/usr/bin:/bin', LANG: 'C.UTF-8'},
  }).trim().split(/\s+/);
  if (browser[0] !== 'Chromium' || browser[1] !== pins.chromium.version) {
    throw new Error('runtime Chromium version mismatch');
  }
  return {contract: runtime.metadata.contract, version: runtime.metadata.version,
    uid: process.geteuid(), gid: process.getegid()};
}
