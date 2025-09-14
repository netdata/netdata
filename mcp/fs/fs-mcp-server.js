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
 * - Read(file, start, lines, headOrTail): read by lines; in 'head' mode return [start, start+lines),
 *   in 'tail' mode treat start as offset from end (start=0 => last lines). Lines are numbered with
 *   4-char right-aligned numbers + a space. File bytes are decoded as UTF-8 with invalid bytes escaped as \u00NN.
 * - Grep(file, regex, before, after, caseSensitive): MULTI-LINE matching ('.' matches newlines). Returns blocks
 *   per match with numbered lines and before/after context. Case sensitivity controlled by flag.
 * - RGrep(dir, regex, caseSensitive): MULTI-LINE matching across files; returns list of matching paths.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const fsp = fs.promises;

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

// -------------------------- Universal file validation --------------------------
/**
 * Universal file validation - checks if a file/path is valid for operations
 * Returns: { valid: boolean, type: string, target?: string, reason?: string, resolvedPath?: string }
 *
 * For symlinks:
 * - allowFollow: if true, validates the target for file operations (Read, Grep)
 * - allowFollow: if false, treats symlinks as invalid for traversal (ListDir, Tree, Find, RGrep)
 */
async function validatePath(absPath, relPath = '', allowFollow = false) {
  try {
    const lstat = await fsp.lstat(absPath);

    // Handle symlinks
    if (lstat.isSymbolicLink()) {
      try {
        const target = await fsp.readlink(absPath);
        const isAbsolute = path.isAbsolute(target);

        // Resolve the target path
        const resolvedTarget = isAbsolute ? target : path.resolve(path.dirname(absPath), target);

        // The ONLY reason to reject a symlink is if the resolved target is outside ROOT
        if (!resolvedTarget.startsWith(ROOT)) {
          let reason = 'symlink target is outside root directory';
          if (isAbsolute) {
            reason = 'symlink points to absolute path outside root directory';
          } else if (target.includes('..')) {
            reason = 'symlink escapes root directory via parent references';
          }
          return {
            valid: false,
            type: 'invalid_link',
            target,
            reason: `symlink ${relPath || absPath} -> ${target} ${reason}`
          };
        }

        // Check if target exists and determine type
        try {
          const targetStat = await fsp.stat(resolvedTarget);
          const targetType = targetStat.isDirectory() ? 'dir' : targetStat.isFile() ? 'file' : 'other';
          const resolvedRel = path.relative(ROOT, resolvedTarget);

          // For file operations, allow following valid symlinks to files
          if (allowFollow && targetType === 'file') {
            return {
              valid: true,
              type: 'file',
              size: targetStat.size || 0,
              isSymlink: true,
              target: resolvedRel,
              resolvedPath: resolvedTarget
            };
          }

          // For listing operations, show symlink info but don't follow
          // Show the original target, not the resolved path
          const displayTarget = target + (targetStat.isDirectory() && !target.endsWith('/') ? '/' : '');
          return {
            valid: false,
            type: 'symlink',
            target: displayTarget,
            targetType,
            targetIsDir: targetStat.isDirectory(),
            reason: allowFollow ? `symlink points to ${targetType}, not a regular file` : 'symlink (not followed for directory operations)'
          };
        } catch (statError) {
          // Check if it's a symlink loop (ELOOP) or just broken
          if (statError.code === 'ELOOP') {
            return {
              valid: false,
              type: 'broken_link',
              target,
              reason: `symlink loop detected: ${relPath || absPath} -> ${target}`
            };
          }
          return {
            valid: false,
            type: 'broken_link',
            target,
            reason: `symlink ${relPath || absPath} -> ${target} (target does not exist)`
          };
        }
      } catch (e) {
        return {
          valid: false,
          type: 'broken_link',
          reason: `cannot read symlink: ${e.message}`
        };
      }
    }

    // Handle special files (FIFO, device, socket, etc.)
    if (!lstat.isFile() && !lstat.isDirectory()) {
      const fileType = lstat.isBlockDevice() ? 'block device' :
                       lstat.isCharacterDevice() ? 'character device' :
                       lstat.isFIFO() ? 'FIFO/named pipe' :
                       lstat.isSocket() ? 'socket' : 'special file';
      return {
        valid: false,
        type: 'special',
        reason: `${fileType} - only regular files and directories are supported`
      };
    }

    // Regular files and directories are valid
    return {
      valid: true,
      type: lstat.isDirectory() ? 'dir' : 'file',
      size: lstat.size || 0
    };
  } catch (error) {
    return {
      valid: false,
      type: 'error',
      reason: `filesystem error: ${error.message}`
    };
  }
}

// Format entry for compact output
function formatEntry(relPath, validation) {
  const name = path.basename(relPath);

  if (validation.type === 'dir') {
    return relPath ? relPath + '/' : './';
  } else if (validation.type === 'file') {
    return relPath || '.';
  } else if (validation.type === 'symlink') {
    return `${relPath} -> ${validation.target}`;
  } else if (validation.type === 'broken_link') {
    return `${relPath} -> [broken]`;
  } else if (validation.type === 'invalid_link') {
    return `${relPath} -> [invalid: ${validation.reason}]`;
  } else if (validation.type === 'special') {
    return `${relPath} [${validation.reason}]`;
  } else {
    return `${relPath} [error]`;
  }
}

async function listDirEntries(dirAbs, baseRel) {
  const entries = [];
  const dirents = await fsp.readdir(dirAbs, { withFileTypes: true });

  for (const d of dirents) {
    const full = path.join(dirAbs, d.name);
    const relPath = (baseRel ? baseRel + '/' : '') + d.name;
    const validation = await validatePath(full, relPath);
    entries.push({ relPath, validation });
  }

  // Sort: directories first, then files, alphabetically within each group
  entries.sort((a, b) => {
    const aIsDir = a.validation.type === 'dir';
    const bIsDir = b.validation.type === 'dir';
    if (aIsDir !== bIsDir) return bIsDir ? 1 : -1;
    return a.relPath.localeCompare(b.relPath);
  });

  return entries;
}

async function walkTree(dirAbs, baseRel, acc, indentStr = '', options = {}, stats = null) {
  if (stats) stats.dirsExamined++;
  const dirents = await fsp.readdir(dirAbs, { withFileTypes: true });
  const entries = [];

  // Collect and validate all entries
  for (const d of dirents) {
    const rel = (baseRel ? baseRel + '/' : '') + d.name;
    const full = path.join(dirAbs, d.name);
    const validation = await validatePath(full, rel);
    entries.push({ name: d.name, rel, full, validation, isDir: d.isDirectory() });

    // Count files
    if (stats && validation.valid && validation.type === 'file') {
      stats.filesCount++;
    }
  }

  // Sort: directories first, then files
  entries.sort((a, b) => {
    if (a.validation.type === 'dir' && b.validation.type !== 'dir') return -1;
    if (a.validation.type !== 'dir' && b.validation.type === 'dir') return 1;
    return a.name.localeCompare(b.name);
  });

  // Process entries
  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i];
    const isLast = i === entries.length - 1;
    const connector = isLast ? '└ ' : '├ ';
    const extension = isLast ? '  ' : '│ ';

    let displayName = formatEntry(entry.name, entry.validation);

    // Add size if requested (only for files, not directories)
    if (options.showSize && entry.validation.type === 'file') {
      try {
        const stat = await fsp.lstat(entry.full);
        displayName += ` (${stat.size}B)`;
      } catch {
        // Ignore stat errors
      }
    }

    // For symlinks in Tree, show WARNING that they're not followed
    if (entry.validation.type === 'symlink' ||
        entry.validation.type === 'broken_link' ||
        entry.validation.type === 'invalid_link') {
      // Get the target (already has trailing slash if directory)
      let target = entry.validation.target || '[broken]';
      if (entry.validation.type === 'invalid_link') {
        target = `[invalid: ${entry.validation.reason}]`;
      }

      acc.push({
        display: indentStr + connector + `WARNING: ${entry.name} -> ${target} not followed`,
        path: entry.rel,
        validation: entry.validation
      });
    } else {
      acc.push({
        display: indentStr + connector + displayName,
        path: entry.rel,
        validation: entry.validation
      });

      // Recurse into valid directories (not symlinks)
      if (entry.validation.valid && entry.validation.type === 'dir') {
        await walkTree(entry.full, entry.rel, acc, indentStr + extension, options, stats);
      }
    }
  }
}

function globToRegex(pattern) {
  // Only '*' and '?'. '*' matches any sequence including '/'; '?' matches single char including '/'.
  const esc = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  const re = '^' + esc.replace(/\*/g, '.*').replace(/\?/g, '.') + '$';
  return new RegExp(re);
}

async function findMatches(baseAbs, baseRel, regex, acc, stats) {
  stats.dirsExamined++;
  const dirents = await fsp.readdir(baseAbs, { withFileTypes: true });
  for (const d of dirents) {
    const rel = (baseRel ? baseRel + '/' : '') + d.name;
    const full = path.join(baseAbs, d.name);
    const validation = await validatePath(full, rel);

    // Count files examined
    if (validation.valid && validation.type === 'file') {
      stats.filesExamined++;
    }

    // Include matches: valid files/dirs AND symlinks (which have valid:false but are still reportable)
    if (regex.test(rel)) {
      // Include if: valid entry OR it's a symlink/broken_link/invalid_link
      if (validation.valid || validation.type === 'symlink' ||
          validation.type === 'broken_link' || validation.type === 'invalid_link') {
        // Store both path and validation for formatting later
        acc.push({ path: rel, validation });
      }
    }

    // Only recurse into valid directories
    if (validation.valid && validation.type === 'dir') {
      await findMatches(full, rel, regex, acc, stats);
    }
  }
}

async function readFileDecoded(abs) { const buf = await fsp.readFile(abs); return decodeUtf8WithHexEscapes(buf); }
function buildRegex(pattern, caseSensitive) { const flags = 'gs' + (caseSensitive ? '' : 'i'); return new RegExp(pattern, flags); }

// -------------------------- Tool implementations --------------------------
async function toolListDir(args) {
  const { dir, showSize, showLastModified, showCreated } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  const abs = resolveRel(dir);

  // Validate the directory itself
  const dirValidation = await validatePath(abs);
  if (!dirValidation.valid || dirValidation.type !== 'dir') {
    throw new Error('not a valid directory');
  }

  const entries = await listDirEntries(abs, dir === '' ? '' : dir.replace(/\\/g, '/'));

  // Count files and directories
  let fileCount = 0;
  let dirCount = 0;
  for (const entry of entries) {
    if (entry.validation.type === 'file') fileCount++;
    else if (entry.validation.type === 'dir') dirCount++;
  }

  // Get additional metadata if requested
  const entriesWithMeta = [];
  for (const entry of entries) {
    const fullPath = path.join(abs, path.basename(entry.relPath));
    let meta = {};

    if (showSize || showLastModified || showCreated) {
      try {
        const stat = await fsp.lstat(fullPath);
        if (showSize) meta.size = stat.size;
        if (showLastModified) meta.mtime = stat.mtime.toISOString().slice(0, 19).replace('T', ' ');
        if (showCreated) meta.ctime = stat.ctime.toISOString().slice(0, 19).replace('T', ' ');
      } catch {
        // If we can't stat, use defaults
        if (showSize) meta.size = 0;
        if (showLastModified) meta.mtime = '-';
        if (showCreated) meta.ctime = '-';
      }
    }

    entriesWithMeta.push({ ...entry, meta });
  }

  // Format output
  const lines = [];
  for (const entry of entriesWithMeta) {
    // Extract just the filename from the relative path
    const name = path.basename(entry.relPath);
    const validation = entry.validation;
    let line = '';

    // Format based on type
    if (validation.type === 'dir') {
      line = name + '/';
    } else if (validation.type === 'file') {
      line = name;
    } else if (validation.type === 'symlink') {
      const target = validation.target;
      line = `${name} -> ${target}`;
    } else if (validation.type === 'broken_link') {
      line = `${name} -> [broken]`;
    } else if (validation.type === 'invalid_link') {
      line = `${name} -> [invalid: ${validation.reason}]`;
    } else if (validation.type === 'special') {
      line = `${name} [${validation.reason}]`;
    } else {
      line = `${name} [error]`;
    }

    // Add metadata if requested (size only for files, not directories)
    const metaParts = [];
    if (showSize && validation.type === 'file') metaParts.push(`${entry.meta.size}B`);
    if (showLastModified) metaParts.push(`mod:${entry.meta.mtime}`);
    if (showCreated) metaParts.push(`cre:${entry.meta.ctime}`);

    if (metaParts.length > 0) {
      line += ` (${metaParts.join(', ')})`;
    }

    lines.push(line);
  }

  // Add summary at the end
  const summary = `\n${fileCount} file${fileCount !== 1 ? 's' : ''} and ${dirCount} director${dirCount !== 1 ? 'ies' : 'y'} in ${dir || 'root'}`;
  lines.push(summary);

  return lines.join('\n');
}

async function toolTree(args) {
  const { dir, showSize } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  // Refuse to list entire tree for empty dir or '.'
  if (dir === '' || dir === '.') {
    throw new Error('listing the entire tree is not allowed due to size limitations. Use ListDir to see all directories and select a directory to list.');
  }
  const abs = resolveRel(dir);

  // Validate the directory itself
  const dirValidation = await validatePath(abs);
  if (!dirValidation.valid || dirValidation.type !== 'dir') {
    throw new Error('not a valid directory');
  }

  const entries = [];
  const stats = { filesCount: 0, dirsExamined: 0 };
  const baseRel = dir.replace(/\\/g, '/');

  // Add the root directory itself (no size for directories)
  const rootDisplay = baseRel + '/';
  entries.push({ display: rootDisplay, path: baseRel, validation: dirValidation });

  await walkTree(abs, baseRel, entries, '', { showSize }, stats);

  // Build output with tree and statistics
  const treeOutput = entries.map(e => e.display).join('\n');
  const summary = `\n${stats.filesCount} file${stats.filesCount !== 1 ? 's' : ''} and ${stats.dirsExamined} director${stats.dirsExamined !== 1 ? 'ies' : 'y'} under ${dir}`;

  return treeOutput + summary;
}

async function toolFind(args) {
  const { dir, glob } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  if (typeof glob !== 'string') throw new Error('glob must be a string');
  const abs = resolveRel(dir);

  // Validate the directory itself
  const dirValidation = await validatePath(abs);
  if (!dirValidation.valid || dirValidation.type !== 'dir') {
    throw new Error('not a valid directory');
  }

  const regex = globToRegex(glob);
  const matches = [];
  const stats = { filesExamined: 0, dirsExamined: 0 };
  const baseDir = dir === '' ? '' : dir.replace(/\\/g, '/');
  await findMatches(abs, baseDir, regex, matches, stats);

  // Format matches, stripping base directory and adding type indicators
  const formattedMatches = matches.map(match => {
    let path = match.path;
    // Strip the base directory from paths
    if (baseDir && path.startsWith(baseDir + '/')) {
      path = path.substring(baseDir.length + 1);
    }

    // Format based on type (similar to ListDir)
    const validation = match.validation;
    if (validation.type === 'dir') {
      path = path + '/';
    } else if (validation.type === 'symlink') {
      const target = validation.target;
      path = `${path} -> ${target}`;
    } else if (validation.type === 'broken_link') {
      path = `${path} -> [broken]`;
    } else if (validation.type === 'invalid_link') {
      path = `${path} -> [invalid: ${validation.reason}]`;
    }

    return path;
  });

  // Build output with statistics
  const summary = `${matches.length} file${matches.length !== 1 ? 's' : ''} matched under ${dir || 'root'}, examined ${stats.filesExamined} file${stats.filesExamined !== 1 ? 's' : ''} in ${stats.dirsExamined} director${stats.dirsExamined !== 1 ? 'ies' : 'y'}`;

  const output = [];

  // Add matches if any
  if (formattedMatches.length > 0) {
    output.push(...formattedMatches);
    output.push('');  // Empty line before summary
  }

  output.push(summary);

  return output.join('\n');
}

async function toolRead(args) {
  const { file, start, lines, headOrTail } = args;
  if (typeof file !== 'string') throw new Error('file must be a string');
  if (!Number.isInteger(start) || start < 0) throw new Error('start must be a non-negative integer');
  if (!Number.isInteger(lines) || lines < 0) throw new Error('lines must be a non-negative integer');
  const mode = headOrTail;
  if (mode !== 'head' && mode !== 'tail') throw new Error("headOrTail must be 'head' or 'tail'");
  const abs = resolveRel(file);

  // Universal validation - allow following symlinks for read operations
  const validation = await validatePath(abs, file, true);
  if (!validation.valid || validation.type !== 'file') {
    // Provide detailed error messages including symlink info
    if (validation.type === 'broken_link') {
      throw new Error(`${file}: broken symlink -> ${validation.target || 'unknown'}`);
    } else if (validation.type === 'invalid_link') {
      throw new Error(`${file}: ${validation.reason}`);
    } else {
      const reason = validation.reason || 'not a regular file';
      throw new Error(reason);
    }
  }

  // Use resolved path if it's a symlink
  const readPath = validation.resolvedPath || abs;
  const text = await readFileDecoded(readPath);
  const arr = splitLinesPreserve(text);
  const total = arr.length;
  let begin = 0; let end = 0;
  if (mode === 'head') { begin = Math.min(start, total); end = Math.min(start + lines, total); }
  else { const skipFromEnd = Math.min(start, total); end = total - skipFromEnd; begin = Math.max(0, end - lines); }
  const numbered = [];
  for (let i = begin; i < end; i++) numbered.push(formatLineNumber(i + 1) + arr[i]);

  // Compact output format: header + content
  const displayPath = validation.isSymlink ? `${file.replace(/\\/g, '/')} -> ${validation.target}` : file.replace(/\\/g, '/');
  const header = `${displayPath} [${mode} ${begin + 1}-${end}/${total}]`;
  return header + '\n' + numbered.join('\n');
}

async function toolGrep(args) {
  const { file, regex, before, after, caseSensitive } = args;
  if (typeof file !== 'string') throw new Error('file must be a string');
  if (typeof regex !== 'string') throw new Error('regex must be a string');
  if (!Number.isInteger(before) || before < 0) throw new Error('before must be a non-negative integer');
  if (!Number.isInteger(after) || after < 0) throw new Error('after must be a non-negative integer');
  const cs = Boolean(caseSensitive);
  const abs = resolveRel(file);

  // Universal validation - allow following symlinks for grep operations
  const validation = await validatePath(abs, file, true);
  if (!validation.valid || validation.type !== 'file') {
    // Provide detailed error messages including symlink info
    if (validation.type === 'broken_link') {
      throw new Error(`${file}: broken symlink -> ${validation.target || 'unknown'}`);
    } else if (validation.type === 'invalid_link') {
      throw new Error(`${file}: ${validation.reason}`);
    } else {
      const reason = validation.reason || 'not a regular file';
      throw new Error(reason);
    }
  }

  // Use resolved path if it's a symlink
  const readPath = validation.resolvedPath || abs;
  const text = await readFileDecoded(readPath);
  const lines = splitLinesPreserve(text);

  // Build line start index for mapping positions to line numbers
  const lineStartIdx = new Array(lines.length);
  let idx = 0;
  for (let i = 0; i < lines.length; i++) {
    lineStartIdx[i] = idx;
    idx += lines[i].length + 1;
  }

  const re = buildRegex(regex, cs);
  const matchData = [];
  let m;

  // Find all matches and their line ranges
  while ((m = re.exec(text)) !== null) {
    const mStart = m.index;
    const mEnd = m.index + (m[0] != null ? m[0].length : 0);

    // Find start and end lines of the match (for multi-line matches)
    let startLine = 0;
    let endLine = lines.length - 1;
    for (let i = 0; i < lines.length; i++) {
      if (lineStartIdx[i] <= mStart) startLine = i;
      else break;
    }
    for (let j = startLine; j < lines.length; j++) {
      if (lineStartIdx[j] < mEnd) endLine = j;
      else break;
    }

    matchData.push({ startLine, endLine });
    if (m[0] === '') re.lastIndex = mEnd + 1;
  }

  // Compact output format with statistics
  const displayPath = validation.isSymlink ? `${file.replace(/\\/g, '/')} -> ${validation.target}` : file.replace(/\\/g, '/');
  const header = `${displayPath}: ${matchData.length} match${matchData.length !== 1 ? 'es' : ''} found. Total lines in file: ${lines.length}`;

  if (matchData.length === 0) {
    return header;
  }

  // Build a map of all lines to display (with context)
  const linesToShow = new Map(); // lineNum -> isMatch
  const matchLines = new Set(); // Track which lines contain matches

  for (const match of matchData) {
    // Mark match lines
    for (let i = match.startLine; i <= match.endLine; i++) {
      matchLines.add(i);
      linesToShow.set(i, true);
    }
    // Add context lines
    for (let i = Math.max(0, match.startLine - before); i < match.startLine; i++) {
      if (!linesToShow.has(i)) linesToShow.set(i, false);
    }
    for (let i = match.endLine + 1; i <= Math.min(lines.length - 1, match.endLine + after); i++) {
      if (!linesToShow.has(i)) linesToShow.set(i, false);
    }
  }

  // Sort line numbers and group into continuous blocks
  const sortedLines = Array.from(linesToShow.keys()).sort((a, b) => a - b);
  const blocks = [];
  let currentBlock = [];

  for (let i = 0; i < sortedLines.length; i++) {
    const lineNum = sortedLines[i];

    if (currentBlock.length === 0) {
      currentBlock.push(lineNum);
    } else if (lineNum === currentBlock[currentBlock.length - 1] + 1) {
      currentBlock.push(lineNum);
    } else {
      // Gap found, save current block and start new one
      blocks.push([...currentBlock]);
      currentBlock = [lineNum];
    }
  }
  if (currentBlock.length > 0) {
    blocks.push(currentBlock);
  }

  // Format output with new arrow format
  const output = [header];

  for (let i = 0; i < blocks.length; i++) {
    const block = blocks[i];

    // Add separator between blocks (except before first block)
    if (i > 0) {
      output.push('───');
    }

    // Output each line in the block
    for (const lineNum of block) {
      const lineNumStr = String(lineNum + 1);
      const isMatch = matchLines.has(lineNum);
      const marker = isMatch ? '→' : ' ';
      output.push(`${lineNumStr}${marker}${lines[lineNum]}`);
    }
  }

  return output.join('\n');
}

async function toolRGrep(args) {
  const { dir, regex, caseSensitive, maxFiles } = args;
  if (typeof dir !== 'string') throw new Error('dir must be a string');
  if (typeof regex !== 'string') throw new Error('regex must be a string');
  // Refuse to grep entire tree for empty dir or '.'
  if (dir === '' || dir === '.') {
    throw new Error('recursive grep on the entire tree is not allowed due to size limitations. Use it on specific subdirectories.');
  }
  const cs = Boolean(caseSensitive);
  const maxFilesLimit = maxFiles || Infinity;
  const abs = resolveRel(dir);

  // Validate the directory itself
  const dirValidation = await validatePath(abs);
  if (!dirValidation.valid || dirValidation.type !== 'dir') {
    throw new Error('not a valid directory');
  }

  const re = buildRegex(regex, cs);
  const hits = [];
  const warnings = [];
  const stack = [[abs, dir === '' ? '' : dir.replace(/\\/g, '/')]];

  // Statistics tracking
  let filesExamined = 0;
  let dirsExamined = 0;

  // Fixed buffer for reading files - 1MB chunk size with 1KB overlap
  const CHUNK_SIZE = 1024 * 1024; // 1MB
  const OVERLAP_SIZE = 1024; // 1KB overlap to catch matches at boundaries
  const buffer = Buffer.allocUnsafe(CHUNK_SIZE + OVERLAP_SIZE);

  fileLoop: while (stack.length > 0) {
    const [curAbs, curRel] = stack.pop();
    dirsExamined++;  // Count each directory we examine
    const dirents = await fsp.readdir(curAbs, { withFileTypes: true });
    for (const d of dirents) {
      const rel = (curRel ? curRel + '/' : '') + d.name;
      const full = path.join(curAbs, d.name);

      // Universal validation
      const validation = await validatePath(full, rel);

      // Collect warnings for symlinks that are not followed
      if (validation.type === 'symlink' ||
          validation.type === 'broken_link' ||
          validation.type === 'invalid_link') {
        let target = validation.target || '[broken]';
        if (validation.type === 'invalid_link') {
          target = `[invalid: ${validation.reason}]`;
        }
        warnings.push(`WARNING: ${rel} -> ${target} not followed`);
        continue;
      }

      // Only process valid directories and files
      if (validation.valid && validation.type === 'dir') {
        stack.push([full, rel]);
        continue;
      }

      if (validation.valid && validation.type === 'file') {
        filesExamined++;  // Count each file we examine
        try {
          // Reset regex lastIndex for each file (g flag maintains state across test() calls)
          re.lastIndex = 0;

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

                // Stop if we've reached maxFiles
                if (hits.length >= maxFilesLimit) {
                  break fileLoop; // Break out of outer loop
                }
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

  // Strip the base directory from matches if we're searching within a subdirectory
  const baseDir = dir === '' ? '' : dir.replace(/\\/g, '/');
  const strippedHits = hits.map(hit => {
    if (baseDir && hit.startsWith(baseDir + '/')) {
      return hit.substring(baseDir.length + 1);
    }
    return hit;
  });

  // Build output with statistics
  const summary = `${hits.length} match${hits.length !== 1 ? 'es' : ''} found in ${hits.length} file${hits.length !== 1 ? 's' : ''} across ${dirsExamined} director${dirsExamined !== 1 ? 'ies' : 'y'} under ${dir || 'root'} (examined ${filesExamined} file${filesExamined !== 1 ? 's' : ''})`;

  const output = [];

  // Add matches if any
  if (strippedHits.length > 0) {
    output.push(...strippedHits);
    output.push('');  // Empty line before summary
  }

  output.push(summary);

  // Add warnings at the end
  if (warnings.length > 0) {
    output.push('');  // Empty line before warnings
    output.push(...warnings);
  }

  return output.join('\n');
}

// -------------------------- MCP methods --------------------------
const tools = [
  { name: 'ListDir', description: 'List contents of a directory under ROOT. Returns entries with optional metadata. Symlinks to valid targets can be followed. All paths are relative to ROOT; ".." segments and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string', description: 'Directory path relative to ROOT (empty string for root, no leading slash)' }, showSize: { type: 'boolean', description: 'Include file sizes in bytes (files only, not directories)' }, showLastModified: { type: 'boolean', description: 'Include last modified timestamp' }, showCreated: { type: 'boolean', description: 'Include creation timestamp' } }, required: ['dir'] }, handler: toolListDir },
  { name: 'Tree', description: 'Recursively list files and directories under a directory (symlinks are not traversed). All paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string', description: 'Directory path relative to ROOT (empty string for root, no leading slash)' }, showSize: { type: 'boolean', description: 'Include file sizes in bytes (files only, not directories)' } }, required: ['dir'] }, handler: toolTree },
  { name: 'Find', description: 'Find entries by simple glob under a directory and its subdirectories. Supported glob: * and ? only (match across path separators). No character classes, braces, or **. Excludes symlinks and special files. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string', description: 'Directory path relative to ROOT (empty string for root, no leading slash)' }, glob: { type: 'string', description: 'Simple glob pattern (* and ? only, matches across path separators)' } }, required: ['dir', 'glob'] }, handler: toolFind },
  { name: 'Read', description: "Read file by lines with numbering. Can follow symlinks to valid files. headOrTail='head' returns lines [start, start+lines); 'tail' treats start as offset from end (start=0 => last lines). Lines are multi-byte aware; invalid UTF-8 bytes are hex-escaped as \\u00NN (strict JSON). Each line is prefixed with a 4-char right-aligned line number and a space.", inputSchema: { type: 'object', properties: { file: { type: 'string', description: 'File path relative to ROOT' }, start: { type: 'integer', minimum: 0, description: 'Starting line (0-based for head, offset from end for tail)' }, lines: { type: 'integer', minimum: 0, description: 'Number of lines to read' }, headOrTail: { type: 'string', enum: ['head', 'tail'], description: "'head' reads from start, 'tail' reads from end" } }, required: ['file', 'start', 'lines', 'headOrTail'] }, handler: toolRead },
  { name: 'Grep', description: 'MULTI-LINE grep in a file. Can follow symlinks to valid files. The regex uses JavaScript syntax with dotAll (. matches newlines). Returns blocks per match with before/after context and line-numbered text. Control case sensitivity via caseSensitive flag. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { file: { type: 'string', description: 'File path relative to ROOT' }, regex: { type: 'string', description: 'JavaScript regular expression pattern' }, before: { type: 'integer', minimum: 0, description: 'Lines of context before match' }, after: { type: 'integer', minimum: 0, description: 'Lines of context after match' }, caseSensitive: { type: 'boolean', description: 'true for case-sensitive matching' } }, required: ['file', 'regex', 'before', 'after', 'caseSensitive'] }, handler: toolGrep },
  { name: 'RGrep', description: 'MULTI-LINE grep across files under a directory. The regex uses JavaScript syntax with dotAll (. matches newlines). Returns paths of files that match. Cannot be used on root directory due to performance. Control case sensitivity via caseSensitive flag. Paths are relative to ROOT; ".." and absolute paths are forbidden.', inputSchema: { type: 'object', properties: { dir: { type: 'string', description: 'Directory path relative to ROOT (cannot be empty/root)' }, regex: { type: 'string', description: 'JavaScript regular expression pattern' }, caseSensitive: { type: 'boolean', description: 'true for case-sensitive matching' }, maxFiles: { type: 'integer', minimum: 1, description: 'Optional: Maximum number of matching files to return' } }, required: ['dir', 'regex', 'caseSensitive'] }, handler: toolRGrep },
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
  // Exit immediately when stdin closes
  process.exit(0);
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
        // Result is already formatted as text/markdown from the tool handlers
        const text = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
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
