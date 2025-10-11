import fs from 'node:fs';
import path from 'node:path';

import { extractBodyWithoutFrontmatter } from './frontmatter.js';

// Resolve ${include:filename} or {{include:filename}} placeholders recursively.
// - baseDir: directory used for resolving relative include paths
// - depth limit prevents infinite recursion
// - refuses to include sensitive files like .env

// no-op placeholder kept for future type-guard reuse (linted if unused)
// eslint-disable-next-line @typescript-eslint/no-unused-vars
function isPlainObject(_val: unknown): _val is Record<string, unknown> { return false; }

function readTextSafe(p: string): string {
  const st = fs.statSync(p);
  if (!st.isFile()) throw new Error(`include target is not a file: ${p}`);
  return fs.readFileSync(p, 'utf-8');
}

function isForbiddenInclude(p: string): boolean {
  const bn = path.basename(p).toLowerCase();
  if (bn === '.env') return true;
  return false;
}

export function resolveIncludes(raw: string, baseDir?: string, maxDepth = 8): string {
  let out = raw;
  const re1 = /\$\{include:([^}]+)\}/g;
  const re2 = /\{\{include:([^}]+)\}\}/g;

  // eslint-disable-next-line functional/no-let
  let depth = 0;
  const expandOnce = (s: string): string => {
    const replaceOne = (src: string, re: RegExp): string => src.replace(re, (_m, group: string) => {
      const rel = group.trim();
      const base = baseDir ?? process.cwd();
      const p = path.resolve(base, rel);
      if (isForbiddenInclude(p)) throw new Error(`including this file is forbidden: ${rel}`);
      try {
        // Read include file, strip its frontmatter body, then resolve nested includes relative to it
        const raw = readTextSafe(p);
        const bodyOnly = extractBodyWithoutFrontmatter(raw);
        const nested = resolveIncludes(bodyOnly, path.dirname(p), Math.max(0, maxDepth - 1));
        return nested;
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        throw new Error(`failed to include '${rel}': ${msg}`);
      }
    });
    let cur = s;
    cur = replaceOne(cur, re1);
    cur = replaceOne(cur, re2);
    return cur;
  };

  // Expand until no changes or depth limit reached
  // eslint-disable-next-line functional/no-loop-statements
  while (depth < maxDepth) {
    const next = expandOnce(out);
    if (next === out) break;
    out = next;
    depth++;
  }
  
  // Check if we still have unresolved includes after reaching max depth
  if (depth >= maxDepth && (re1.test(out) || re2.test(out))) {
    throw new Error(`Maximum include depth (${String(maxDepth)}) exceeded - possible circular reference or deeply nested includes`);
  }
  
  return out;
}
