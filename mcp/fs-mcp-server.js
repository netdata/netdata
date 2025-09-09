#!/usr/bin/env node
/**
 * Minimal MCP stdio server (no external deps).
 *
 * Usage: node fs-mcp-server.js <ROOT_DIR>
 * - All tool paths are relative to <ROOT_DIR>
 * - '..' segments are forbidden in any input path
 * - Absolute paths are not allowed
 * - Symlinks are listed as type 'symlink' but never followed for traversal
 *
 * Tools
 * - ListDir(dir): list children of dir (relative to ROOT), include type and size (bytes)
 * - Tree(dir): recursively list files/dirs under dir (symlinks not traversed)
 * - Find(dir, glob): recursively find entries whose relative path matches simple glob
 *   Limitations: only '*' and '?' are supported and match across '/'; no character classes, braces, or '**'
 * - Read(file, start, lines, head_or_tail): read by lines; in 'head' mode return [start, start+lines),
 *   in 'tail' mode treat start as offset from end (start=0 => last lines). Lines are numbered with
 *   4-char right-aligned numbers + a space. File bytes are decoded as UTF-8 with invalid bytes escaped as \u00NN.
 * - Grep(file, regex, before, after, caseSensitive): MULTI-LINE matching ('.' matches newlines). Returns blocks
 *   per match with numbered lines and before/after context. Case sensitivity controlled by flag.
 * - RGrep(dir, regex, caseSensitive): MULTI-LINE matching across files; returns list of matching paths.
 */

'use strict';

const fs = require('node:fs');
const fsp = fs.promises;
const path = require('node:path');

// -------------------------- Utility: JSON-RPC over LSP framing --------------------------
const STDIN = process.stdin;
const STDOUT = process.stdout;

function send(message) {
  const json = JSON.stringify(message);
  // MCP SDK uses newline-delimited JSON, not LSP framing
  const payload = json + '\n';
  STDOUT.write(payload);
}

function respond(id, result) { send({ jsonrpc: '2.0', id, result }); }
function respondError(id, code, message, data) {
  const error = data === undefined ? { code, message } : { code, message, data };
  send({ jsonrpc: '2.0', id, error });
}

// -------------------------- Root and path safety --------------------------
const ROOT = (() => {
  // console.error('MCP: Starting fs-mcp-server.js with args:', process.argv.slice(2));
  const arg = process.argv[2];
  if (!arg || typeof arg !== 'string') {
    console.error('Usage: node fs-mcp-server.js <ROOT_DIR>');
    process.exit(1);
  }
  const abs = path.resolve(arg);
  let stat;
  try { stat = fs.statSync(abs); } catch (e) {
    console.error(`Root path not found: ${abs}`);
    process.exit(1);
  }
  if (!stat.isDirectory()) {
    console.error(`Root path is not a directory: ${abs}`);
    process.exit(1);
  }
  return abs;
})();

function assertNoDotDot(raw) {
  if (typeof raw !== 'string') throw new Error('path must be a string');
  if (raw === '') return;
  if (raw.startsWith('/') || raw.startsWith('\\') || /^[A-Za-z]:[\\/]/.test(raw)) {
    throw new Error('absolute paths are not allowed');
  }
  const parts = raw.split(/[\\/]+/);
  for (const p of parts) { if (p === '..') throw new Error('".." is not allowed'); }
}
function resolveRel(raw) {
  assertNoDotDot(raw);
  const rel = raw === '' ? '' : raw.replace(/\\/g, '/');
  return path.join(ROOT, rel);
}

// -------------------------- Binary-safe UTF-8 decode with hex escapes --------------------------
function decodeUtf8WithHexEscapes(buf) {
  const bytes = buf;
  let out = '';
  for (let i = 0; i < bytes.length; i++) {
    const b0 = bytes[i];
    if (b0 <= 0x7F) { out += String.fromCharCode(b0); continue; }
    let needed = 0; let codePoint = 0;
    if (b0 >= 0xC2 && b0 <= 0xDF) { needed = 1; codePoint = b0 & 0x1F; }
    else if (b0 >= 0xE0 && b0 <= 0xEF) { needed = 2; codePoint = b0 & 0x0F; }
    else if (b0 >= 0xF0 && b0 <= 0xF4) { needed = 3; codePoint = b0 & 0x07; }
    else { out += `\\u${b0.toString(16).padStart(4, '0')}`; continue; }
    if (i + needed >= bytes.length) { out += `\\u${b0.toString(16).padStart(4, '0')}`; continue; }
    let valid = true;
    for (let j = 1; j <= needed; j++) {
      const bj = bytes[i + j];
      if ((bj & 0xC0) !== 0x80) { valid = false; break; }
      codePoint = (codePoint << 6) | (bj & 0x3F);
    }
    if (!valid) { out += `\\u${b0.toString(16).padStart(4, '0')}`; continue; }
    if (codePoint >= 0xD800 && codePoint <= 0xDFFF) { out += `\\u${b0.toString(16).padStart(4, '0')}`; continue; }
    if (codePoint <= 0xFFFF) { out += String.fromCharCode(codePoint); }
    else { codePoint -= 0x10000; out += String.fromCharCode(0xD800 + (codePoint >> 10)); out += String.fromCharCode(0xDC00 + (codePoint & 0x3FF)); }
    i += needed;
  }
  return out;
}

// -------------------------- Common helpers --------------------------
function formatLineNumber(n) { return String(n).padStart(4, ' ') + ' '; }
function splitLinesPreserve(str) { const raw = str.split('\n'); return raw.map((s) => (s.endsWith('\r') ? s.slice(0, -1) : s)); }

async function listDirEntries(dirAbs, baseRel) {
  const out = [];
  const dirents = await fsp.readdir(dirAbs, { withFileTypes: true });
  for (const d of dirents) {
    const full = path.join(dirAbs, d.name);
    let st; try { st = await fsp.lstat(full); } catch { continue; }
    const type = d.isDirectory() ? 'dir' : d.isSymbolicLink() ? 'symlink' : d.isFile() ? 'file' : 'other';
    out.push({ path: (baseRel ? baseRel + '/' : '') + d.name, name: d.name, type, size: typeof st.size === 'number' ? st.size : 0 });
  }
  return out;
}

async function walkTree(dirAbs, baseRel, acc) {
  const dirents = await fsp.readdir(dirAbs, { withFileTypes: true });
  for (const d of dirents) {
    const rel = (baseRel ? baseRel + '/' : '') + d.name;
    const full = path.join(dirAbs, d.name);
    let st; try { st = await fsp.lstat(full); } catch { continue; }
    const type = d.isDirectory() ? 'dir' : d.isSymbolicLink() ? 'symlink' : d.isFile() ? 'file' : 'other';
    acc.push({ path: rel, type, size: typeof st.size === 'number' ? st.size : 0 });
    if (d.isDirectory()) await walkTree(full, rel, acc);
  }
}

function globToRegex(pattern) {
  // Only '*' and '?'. '*' matches any sequence including '/'; '?' matches single char including '/'.
  const esc = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  const re = '^' + esc.replace(/\*/g, '.*').replace(/\?/g, '.') + '$';
  return new RegExp(re);
}

async function findMatches(baseAbs, baseRel, regex, acc) {
  const dirents = await fsp.readdir(baseAbs, { withFileTypes: true });
  for (const d of dirents) {
    const rel = (baseRel ? baseRel + '/' : '') + d.name;
    const full = path.join(baseAbs, d.name);
    let st; try { st = await fsp.lstat(full); } catch { continue; }
    if (regex.test(rel)) acc.push(rel);
    if (d.isDirectory()) await findMatches(full, rel, regex, acc);
  }
}

async function readFileDecoded(abs) { const buf = await fsp.readFile(abs); return decodeUtf8WithHexEscapes(buf); }
function buildRegex(pattern, caseSensitive) { const flags = 'gs' + (caseSensitive ? '' : 'i'); return new RegExp(pattern, flags); }

// -------------------------- Tool implementations --------------------------
async function toolListDir(args) {
  const { dir } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  const abs = resolveRel(dir);
  const st = await fsp.stat(abs);
  if (!st.isDirectory()) throw new Error('not a directory');
  const entries = await listDirEntries(abs, dir === '' ? '' : dir.replace(/\\/g, '/'));
  return { entries };
}

async function toolTree(args) {
  const { dir } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  // Refuse to list entire tree for empty dir or '.'
  if (dir === '' || dir === '.') {
    throw new Error('listing the entire tree is not allowed due to size limitations. Use ListDir to see all directories and select a directory to list.');
  }
  const abs = resolveRel(dir);
  const st = await fsp.stat(abs);
  if (!st.isDirectory()) throw new Error('not a directory');
  const out = [];
  await walkTree(abs, dir === '' ? '' : dir.replace(/\\/g, '/'), out);
  return { entries: out };
}

async function toolFind(args) {
  const { dir, glob } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  if (typeof glob !== 'string') throw new Error('glob must be a string');
  const abs = resolveRel(dir);
  const st = await fsp.stat(abs);
  if (!st.isDirectory()) throw new Error('not a directory');
  const regex = globToRegex(glob);
  const out = [];
  await findMatches(abs, dir === '' ? '' : dir.replace(/\\/g, '/'), regex, out);
  return { paths: out };
}

async function toolRead(args) {
  const { file, start, lines, head_or_tail } = args;
  if (typeof file !== 'string') throw new Error('file must be a string');
  if (!Number.isInteger(start) || start < 0) throw new Error('start must be a non-negative integer');
  if (!Number.isInteger(lines) || lines < 0) throw new Error('lines must be a non-negative integer');
  const mode = head_or_tail;
  if (mode !== 'head' && mode !== 'tail') throw new Error("head_or_tail must be 'head' or 'tail'");
  const abs = resolveRel(file);
  const st = await fsp.stat(abs);
  if (!st.isFile()) throw new Error('not a file');
  const text = await readFileDecoded(abs);
  const arr = splitLinesPreserve(text);
  const total = arr.length;
  let begin = 0; let end = 0;
  if (mode === 'head') { begin = Math.min(start, total); end = Math.min(start + lines, total); }
  else { const skipFromEnd = Math.min(start, total); end = total - skipFromEnd; begin = Math.max(0, end - lines); }
  const numbered = [];
  for (let i = begin; i < end; i++) numbered.push(formatLineNumber(i + 1) + arr[i]);
  return { file: file.replace(/\\/g, '/'), start, lines, mode, total, output: numbered.join('\n') };
}

async function toolGrep(args) {
  const { file, regex, before, after, caseSensitive } = args;
  if (typeof file !== 'string') throw new Error('file must be a string');
  if (typeof regex !== 'string') throw new Error('regex must be a string');
  if (!Number.isInteger(before) || before < 0) throw new Error('before must be a non-negative integer');
  if (!Number.isInteger(after) || after < 0) throw new Error('after must be a non-negative integer');
  const cs = Boolean(caseSensitive);
  const abs = resolveRel(file);
  const st = await fsp.stat(abs);
  if (!st.isFile()) throw new Error('not a file');
  const text = await readFileDecoded(abs);
  const lines = splitLinesPreserve(text);
  const lineStartIdx = new Array(lines.length);
  let idx = 0;
  for (let i = 0; i < lines.length; i++) { lineStartIdx[i] = idx; idx += lines[i].length + 1; }
  const re = buildRegex(regex, cs);
  const matches = [];
  let m;
  while ((m = re.exec(text)) !== null) {
    const mStart = m.index; const mEnd = m.index + (m[0] != null ? m[0].length : 0);
    let startLine = 0; let endLine = lines.length - 1;
    for (let i = 0; i < lines.length; i++) { if (lineStartIdx[i] <= mStart) startLine = i; else break; }
    for (let j = startLine; j < lines.length; j++) { if (lineStartIdx[j] < mEnd) endLine = j; else break; }
    const from = Math.max(0, startLine - before); const to = Math.min(lines.length - 1, endLine + after);
    const block = []; for (let k = from; k <= to; k++) block.push(formatLineNumber(k + 1) + lines[k]);
    matches.push({ startLine: startLine + 1, endLine: endLine + 1, snippet: block.join('\n') });
    if (m[0] === '') re.lastIndex = mEnd + 1;
  }
  return { file: file.replace(/\\/g, '/'), multiLine: true, caseSensitive: cs, matches };
}

async function toolRGrep(args) {
  const { dir, regex, caseSensitive } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  if (typeof regex !== 'string') throw new Error('regex must be a string');
  // Refuse to grep entire tree for empty dir or '.'
  if (dir === '' || dir === '.') {
    throw new Error('recursive grep on the entire tree is not allowed due to size limitations. Use it on specific subdirectories.');
  }
  const cs = Boolean(caseSensitive);
  const abs = resolveRel(dir);
  const st = await fsp.stat(abs);
  if (!st.isDirectory()) throw new Error('not a directory');
  const re = buildRegex(regex, cs);
  const hits = [];
  const stack = [[abs, dir === '' ? '' : dir.replace(/\\/g, '/')]];
  
  // Fixed buffer for reading files - 1MB chunk size with 1KB overlap
  const CHUNK_SIZE = 1024 * 1024; // 1MB
  const OVERLAP_SIZE = 1024; // 1KB overlap to catch matches at boundaries
  const buffer = Buffer.allocUnsafe(CHUNK_SIZE + OVERLAP_SIZE);
  
  while (stack.length > 0) {
    const [curAbs, curRel] = stack.pop();
    const dirents = await fsp.readdir(curAbs, { withFileTypes: true });
    for (const d of dirents) {
      const rel = (curRel ? curRel + '/' : '') + d.name;
      const full = path.join(curAbs, d.name);
      let st2; try { st2 = await fsp.lstat(full); } catch { continue; }
      if (d.isDirectory()) { stack.push([full, rel]); continue; }
      if (d.isSymbolicLink()) { continue; }
      if (d.isFile()) {
        try {
          // Check file in chunks to avoid loading entire file into memory
          const fd = await fsp.open(full, 'r');
          try {
            let position = 0;
            let previousOverlap = '';
            let found = false;
            
            while (!found) {
              const { bytesRead } = await fd.read(buffer, 0, CHUNK_SIZE, position);
              if (bytesRead === 0) break;
              
              // Convert chunk to string with previous overlap
              const chunk = decodeUtf8WithHexEscapes(buffer.subarray(0, bytesRead));
              const searchText = previousOverlap + chunk;
              
              if (re.test(searchText)) {
                hits.push(rel);
                found = true;
                break;
              }
              
              // If we read a full chunk, save overlap for next iteration
              if (bytesRead === CHUNK_SIZE) {
                // Keep last OVERLAP_SIZE chars for next iteration
                const fullText = previousOverlap + chunk;
                previousOverlap = fullText.slice(-OVERLAP_SIZE);
                position += bytesRead;
              } else {
                // Last chunk, no more to read
                break;
              }
            }
          } finally {
            await fd.close();
          }
        } catch { }
      }
    }
  }
  return { paths: hits, multiLine: true, caseSensitive: cs };
}

// -------------------------- MCP methods --------------------------
const tools = [
  { name: 'ListDir', description: 'List contents of a directory under ROOT. Returns entries with path, type, and size (bytes). All paths are relative to ROOT; ".." segments and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string' } }, required: ['dir'] }, handler: toolListDir },
  { name: 'Tree', description: 'Recursively list files and directories under a directory (symlinks are not traversed). All paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string' } }, required: ['dir'] }, handler: toolTree },
  { name: 'Find', description: 'Find entries by simple glob under a directory and its subdirectories. Supported glob: * and ? only (match across path separators). No character classes, braces, or **. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string' }, glob: { type: 'string' } }, required: ['dir', 'glob'] }, handler: toolFind },
  { name: 'Read', description: "Read file by lines with numbering. head_or_tail='head' returns lines [start, start+lines); 'tail' treats start as offset from end (start=0 => last lines). Lines are multi-byte aware; invalid UTF-8 bytes are hex-escaped as \\u00NN (strict JSON). Each line is prefixed with a 4-char right-aligned line number and a space.", inputSchema: { type: 'object', properties: { file: { type: 'string' }, start: { type: 'integer', minimum: 0 }, lines: { type: 'integer', minimum: 0 }, head_or_tail: { type: 'string', enum: ['head', 'tail'] } }, required: ['file', 'start', 'lines', 'head_or_tail'] }, handler: toolRead },
  { name: 'Grep', description: 'MULTI-LINE grep in a file. The regex uses dotAll so . matches newlines. Returns blocks per match with before/after context and line-numbered text. Control case sensitivity via caseSensitive flag. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { file: { type: 'string' }, regex: { type: 'string' }, before: { type: 'integer', minimum: 0 }, after: { type: 'integer', minimum: 0 }, caseSensitive: { type: 'boolean' } }, required: ['file', 'regex', 'before', 'after', 'caseSensitive'] }, handler: toolGrep },
  { name: 'RGrep', description: 'MULTI-LINE grep across files under a directory. The regex uses dotAll so . matches newlines. Returns paths of files that match. Control case sensitivity via caseSensitive flag. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string' }, regex: { type: 'string' }, caseSensitive: { type: 'boolean' } }, required: ['dir', 'regex', 'caseSensitive'] }, handler: toolRGrep },
];

function listToolsResponse() { return { tools: tools.map((t) => ({ name: t.name, description: t.description, inputSchema: t.inputSchema })) }; }

// -------------------------- Message loop --------------------------
let buffer = Buffer.alloc(0);

STDIN.on('data', (chunk) => { 
  // console.error('MCP: Received stdin chunk:', chunk.length, 'bytes');
  buffer = Buffer.concat([buffer, chunk]); 
  processBuffer(); 
});
STDIN.on('end', () => { 
  // console.error('MCP: stdin closed');
  // Process any remaining messages before exiting
  processBuffer();
  // if (buffer.length > 0) {
  //   console.error('MCP: Unprocessed buffer on exit:', buffer.length, 'bytes:', buffer.toString('utf8').substring(0, 200));
  // }
  // Give stdout time to flush before exiting
  setImmediate(() => process.exit(0));
});

function processBuffer() {
  // Check if buffer starts with JSON (no Content-Length header)
  const bufStr = buffer.toString('utf8');
  if (bufStr.startsWith('{')) {
    // Try to parse as newline-delimited JSON
    const lines = bufStr.split('\n');
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      if (line === '') continue;
      
      try {
        const msg = JSON.parse(line);
        // console.error('MCP: Parsed raw JSON message');
        handleMessage(msg);
        // Remove processed line from buffer
        const lineEnd = bufStr.indexOf('\n', bufStr.indexOf(line)) + 1;
        buffer = buffer.subarray(lineEnd > 0 ? lineEnd : line.length);
      } catch (e) {
        // Not a complete JSON line yet, wait for more data
        if (i === lines.length - 1 && !line.endsWith('}')) {
          // Last line is incomplete, keep it in buffer
          return;
        }
        console.error('MCP: Failed to parse line as JSON:', e.message, 'Line:', line.substring(0, 100));
      }
    }
    return;
  }
  
  // Parse headers (LSP-style framing)
  while (true) {
    const headerEnd = buffer.indexOf('\r\n\r\n');
    if (headerEnd === -1) {
      // Also check for LF-only separator for compatibility
      const headerEndLF = buffer.indexOf('\n\n');
      if (headerEndLF === -1) {
        // Check if we have unparseable data
        if (buffer.length > 0 && buffer.length < 8192) {
          const preview = buffer.subarray(0, Math.min(100, buffer.length)).toString('utf8').replace(/[\r\n]/g, '\\n');
          console.error('MCP: Waiting for header, buffer has:', buffer.length, 'bytes, preview:', preview);
        }
        return;
      }
      const header = buffer.subarray(0, headerEndLF).toString('utf8');
      const match = /Content-Length:\s*(\d+)/i.exec(header);
      if (!match) { 
        console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
        buffer = buffer.subarray(headerEndLF + 2); 
        continue; 
      }
      const length = parseInt(match[1], 10);
      const total = headerEndLF + 2 + length;
      if (buffer.length < total) return;
      const body = buffer.subarray(headerEndLF + 2, total).toString('utf8');
      buffer = buffer.subarray(total);
      let msg; 
      try { 
        msg = JSON.parse(body); 
      } catch (e) { 
        console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
        continue; 
      }
      handleMessage(msg);
      continue;
    }
    const sepLen = 4;

    const header = buffer.subarray(0, headerEnd).toString('utf8');
    const match = /Content-Length:\s*(\d+)/i.exec(header);
    if (!match) { 
      console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
      buffer = buffer.subarray(headerEnd + sepLen); 
      continue; 
    }
    const length = parseInt(match[1], 10);
    const total = headerEnd + sepLen + length;
    if (buffer.length < total) return;
    const body = buffer.subarray(headerEnd + sepLen, total).toString('utf8');
    buffer = buffer.subarray(total);
    let msg; 
    try { 
      msg = JSON.parse(body); 
    } catch (e) { 
      console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
      continue; 
    }
    handleMessage(msg);
  }
}

function handleMessage(msg) {
  const { id, method, params } = msg;
  // console.error('MCP server received:', method, JSON.stringify(params));
  if (method === 'initialize') {
    const result = { protocolVersion: '2024-11-05', serverInfo: { name: 'fs-mcp', version: '0.1.2' }, capabilities: { tools: {} } };
    respond(id, result); return;
  }
  if (method === 'tools/list') { respond(id, listToolsResponse()); return; }
  if (method === 'tools/call') {
    const { name, arguments: args } = params || {};
    const tool = tools.find((t) => t.name === name);
    if (!tool) { respondError(id, -32601, `Unknown tool: ${name}`); return; }
    (async () => {
      try {
        const result = await tool.handler(args || {});
        const text = (() => { try { return JSON.stringify(result, null, 2); } catch { return String(result); } })();
        respond(id, { content: [{ type: 'text', text }] });
      } catch (e) {
        const message = e && typeof e.message === 'string' ? e.message : 'Tool error';
        respondError(id, -32000, message);
      }
    })();
    return;
  }
  if (method === 'notifications/initialized') { return; }
  if (method === 'notifications/cancelled') { return; }  // Silently ignore cancellation notifications
  if (method === 'prompts/list') { respond(id, { prompts: [] }); return; }
  if (method === 'prompts/get') { respond(id, { prompt: null }); return; }
  if (method === 'resources/list') { respond(id, { resources: [] }); return; }
  if (method === 'resources/read') { respond(id, { contents: [] }); return; }
  if (method === 'logging/setLevel') { respond(id, {}); return; }
  if (method === 'ping') { respond(id, {}); return; }
  
  // Log unknown methods
  console.error('MCP: Unknown method:', String(method), 'id:', id, 'params:', JSON.stringify(params));
  if (id !== undefined) respondError(id, -32601, `Unknown method: ${String(method)}`);
}

// console.error('MCP: Server ready, waiting for input on stdin');
STDIN.resume();
