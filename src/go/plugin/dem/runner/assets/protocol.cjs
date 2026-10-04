// SPDX-License-Identifier: GPL-3.0-or-later
'use strict';
const fs = require('node:fs');
const path = require('node:path');

// The wire text budget matches the Go diagnosis contract; it is not a run limit.
let secretPattern;
function configureSecrets(values = {}) {
  const known = [...new Set(Object.values(values).filter(value => typeof value === 'string' && value))].sort((a, b) => b.length - a.length);
  secretPattern = known.length ? new RegExp(known.map(value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|'), 'g') : undefined;
}
configureSecrets(Object.fromEntries(Object.entries(process.env).filter(([name]) => /^DEM_SECRET_[A-Za-z0-9_]+$/.test(name))));
function text(value) {
  const raw = String(value ?? '');
  // Clipping first could expose a secret prefix that Go cannot match exactly.
  const redacted = secretPattern ? raw.replace(secretPattern, '[REDACTED]') : raw;
  return Array.from(redacted).slice(0, 2000).join('');
}
const event = (kind, values = {}) => ({kind, at_ms: Date.now(), ...values});
function write(fd, record) {
  fs.writeSync(fd, JSON.stringify(record) + '\n');
}
function captureState(enabled, status, artifacts) {
  if (!enabled) return 'disabled';
  if (artifacts.length) return 'available';
  return ['failed', 'timeout', 'error'].includes(status) ? 'unavailable' : 'not_needed';
}
function screenshotCandidate(file, workDir, index) {
  const actualWork = fs.realpathSync(workDir);
  const output = fs.realpathSync(path.join(actualWork, 'output'));
  const resolved = fs.realpathSync(file);
  const relative = path.relative(output, resolved);
  if (!relative || relative.startsWith('..' + path.sep) || relative === '..' || path.isAbsolute(relative)) return null;
  const fd = fs.openSync(resolved, 'r');
  const header = Buffer.alloc(8);
  try {
    if (!fs.fstatSync(fd).isFile() || fs.readSync(fd, header, 0, 8, 0) !== 8 ||
        !header.equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) return null;
  } finally { fs.closeSync(fd); }
  return {id: 'screenshot-' + index, kind: 'screenshot', path: path.relative(actualWork, resolved), mime: 'image/png'};
}
module.exports = {text, event, write, captureState, screenshotCandidate, configureSecrets};
