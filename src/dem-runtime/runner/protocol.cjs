// SPDX-License-Identifier: GPL-3.0-or-later
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {StringDecoder} = require('node:string_decoder');

// The wire text budget matches the Go diagnosis contract; it is not a run limit.
let secretPattern;
let known = [];
let longestSecret = 1;
function configureSecrets(values = {}) {
  known = [...new Set(Object.values(values).filter(value => typeof value === 'string' && value))].sort((a, b) => b.length - a.length);
  longestSecret = known.reduce((longest, value) => Math.max(longest, value.length), 1);
  secretPattern = known.length ? new RegExp(known.map(value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|'), 'g') : undefined;
}
configureSecrets(Object.fromEntries(Object.entries(process.env).filter(([name]) => /^DEM_SECRET_[A-Za-z0-9_]+$/.test(name))));
function text(value) {
  const raw = String(value ?? '');
  // Clipping first could expose a secret prefix that Go cannot match exactly.
  const redacted = secretPattern ? raw.replace(secretPattern, '[REDACTED]') : raw;
  return Array.from(redacted).slice(0, 2000).join('');
}

// Hold enough original input to decide a match before emitting any of its bytes.
// State is bounded by the longest supplied secret (itself within the request cap).
// Stdout/stderr are streams, not independently safe reporter callback strings.
function secretStream(emit) {
  const decoder = new StringDecoder('utf8');
  let pending = '', ended = false;
  function output(value) {
    // Do not clip: downstream pattern redaction must see the complete stream.
    const chars = Array.from(value);
    for (let i = 0; i < chars.length; i += 2000) emit(chars.slice(i, i + 2000).join(''));
  }
  function drain(final) {
    let safe = final ? pending.length : Math.max(0, pending.length - longestSecret + 1);
    if (!final && safe && /[\uD800-\uDBFF]/.test(pending[safe - 1])) safe--;
    const partial = final ? known.reduce((longest, secret) => Math.max(longest, prefixSuffix(pending, secret)), 0) : 0;
    const partialStart = pending.length - partial;
    let consumed = 0, out = '';
    if (secretPattern) {
      secretPattern.lastIndex = 0;
      for (let match; (match = secretPattern.exec(pending)) && match.index < Math.min(safe, partialStart);) {
        out += pending.slice(consumed, match.index) + '[REDACTED]';
        consumed = match.index + match[0].length;
        if (partial && consumed > partialStart) { consumed = pending.length; break; }
      }
    }
    const end = Math.max(safe, consumed);
    let tail = pending.slice(consumed, end);
    if (partial && consumed <= partialStart) tail = pending.slice(consumed, partialStart) + '[REDACTED]';
    output(out + tail);
    pending = pending.slice(end);
  }
  function append(value) {
    for (let i = 0; i < value.length; i += 4096) {
      pending += value.slice(i, i + 4096);
      drain(false);
    }
  }
  return {
    write(chunk) {
      if (ended) throw new Error('diagnostic stream already ended');
      append(decoder.write(Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk))));
    },
    end() { if (!ended) { ended = true; append(decoder.end()); drain(true); } },
  };
}
// Longest proper secret prefix at EOF: withhold it even after forced termination.
// KMP keeps repeated-prefix values linear rather than testing every suffix.
function prefixSuffix(value, secret) {
  if (!value.length || secret.length < 2) return 0;
  const prefix = new Uint32Array(secret.length);
  for (let i = 1, n = 0; i < secret.length; i++) {
    while (n && secret[i] !== secret[n]) n = prefix[n - 1];
    if (secret[i] === secret[n]) n++;
    prefix[i] = n;
  }
  let n = 0;
  for (const char of value.slice(-Math.max(0, secret.length - 1)).split('')) {
    while (n && char !== secret[n]) n = prefix[n - 1];
    if (char === secret[n]) n++;
  }
  return n;
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
module.exports = {text, event, write, captureState, screenshotCandidate, configureSecrets, secretStream};
