import { readFileSync, statSync } from 'node:fs';
import path from 'node:path';

import { Liquid, type Template } from 'liquidjs';

export interface LoadedTemplate {
  name: string;
  filePath: string;
  source: string;
  parsed: Template[];
}

export interface TemplateEngine {
  engine: Liquid;
  templates: Record<string, string>;
}

const INCLUDE_TAG_REGEX = /\{%-?\s*(?:render|include)\s+(['"])([^'"]+)\1[^%]*-?%\}/g;
const INCLUDE_ANY_TAG_REGEX = /\{%-?\s*(render|include)\s+([^%]+)-?%\}/g;
const DEFAULT_MAX_DEPTH = 8;

const isForbiddenInclude = (filePath: string): boolean => (
  path.basename(filePath).toLowerCase() === '.env'
);

const ensureFileExists = (filePath: string, includeRef: string): void => {
  try {
    const stat = statSync(filePath);
    if (!stat.isFile()) {
      throw new Error(`include target is not a file: ${filePath}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`include file not found: ${includeRef} (${message})`);
  }
};

const assertNoDynamicIncludes = (source: string): void => {
  const matches = Array.from(source.matchAll(INCLUDE_ANY_TAG_REGEX));
  matches.forEach((match) => {
    const args = typeof match[2] === 'string' ? match[2].trim() : '';
    if (args.startsWith('"') || args.startsWith("'")) return;
    throw new Error(`Liquid include/render must use a static quoted path: ${match[0].trim()}`);
  });
};

const resolveIncludePath = (
  includeRef: string,
  filePath: string,
): string => {
  if (path.isAbsolute(includeRef)) {
    return includeRef;
  }
  const baseDir = path.dirname(filePath);
  return path.resolve(baseDir, includeRef);
};

const rewriteIncludeRefs = (
  source: string,
  filePath: string,
  rootDir: string,
): { rewritten: string; includes: string[] } => {
  assertNoDynamicIncludes(source);
  const includes: string[] = [];
  const rewritten = source.replace(INCLUDE_TAG_REGEX, (match, _quote, includeRef) => {
    const rawRef = typeof includeRef === 'string' ? includeRef : '';
    const normalizedRef = rawRef.trim();
    const resolved = resolveIncludePath(normalizedRef, filePath);
    if (isForbiddenInclude(resolved)) {
      throw new Error(`including this file is forbidden: ${normalizedRef}`);
    }
    ensureFileExists(resolved, normalizedRef);
    includes.push(resolved);
    const templateKey = templateKeyFromFilePath(rootDir, resolved);
    return match.replace(rawRef, templateKey);
  });
  return { rewritten, includes };
};

const toPosixPath = (value: string): string => value.split(path.sep).join('/');

export const templateKeyFromFilePath = (rootDir: string, filePath: string): string => {
  const relative = path.relative(rootDir, filePath);
  const normalized = toPosixPath(relative);
  if (normalized.length > 0 && !normalized.startsWith('..')) {
    return normalized;
  }
  return toPosixPath(path.resolve(filePath));
};

export const collectTemplateSources = (
  rootDir: string,
  entryFiles: string[],
): Record<string, string> => {
  const templates: Record<string, string> = {};
  const visited = new Set<string>();

  const visit = (filePath: string, depth = 0): void => {
    if (visited.has(filePath)) return;
    if (depth > DEFAULT_MAX_DEPTH) {
      throw new Error(`Maximum include depth (${String(DEFAULT_MAX_DEPTH)}) exceeded while resolving ${filePath}`);
    }
    visited.add(filePath);
    const source = readFileSync(filePath, 'utf-8');
    const { rewritten, includes } = rewriteIncludeRefs(source, filePath, rootDir);
    const templateKey = templateKeyFromFilePath(rootDir, filePath);
    templates[templateKey] = rewritten;
    includes.forEach((resolved) => {
      visit(resolved, depth + 1);
    });
  };

  entryFiles.forEach((entry) => {
    const resolved = path.resolve(rootDir, entry);
    visit(resolved);
  });

  return templates;
};

export const collectTemplateSourcesFromContent = (
  rootDir: string,
  entry: { filePath: string; source: string },
): Record<string, string> => {
  const templates: Record<string, string> = {};
  const visited = new Set<string>();

  const visit = (filePath: string, sourceOverride?: string, depth = 0): void => {
    if (visited.has(filePath)) return;
    if (depth > DEFAULT_MAX_DEPTH) {
      throw new Error(`Maximum include depth (${String(DEFAULT_MAX_DEPTH)}) exceeded while resolving ${filePath}`);
    }
    visited.add(filePath);
    const source = sourceOverride ?? readFileSync(filePath, 'utf-8');
    const { rewritten, includes } = rewriteIncludeRefs(source, filePath, rootDir);
    const templateKey = templateKeyFromFilePath(rootDir, filePath);
    templates[templateKey] = rewritten;
    includes.forEach((resolved) => {
      visit(resolved, undefined, depth + 1);
    });
  };

  visit(entry.filePath, entry.source);
  return templates;
};

export const createTemplateEngine = (templates: Record<string, string>): TemplateEngine => {
  const engine = new Liquid({
    templates,
    extname: '',
    cache: false,
    strictFilters: true,
    strictVariables: true,
  });

  engine.registerFilter('json', (value: unknown) => JSON.stringify(value ?? null));
  engine.registerFilter('json_pretty', (value: unknown) => JSON.stringify(value ?? null, null, 2));

  return { engine, templates };
};

export const loadTemplate = (
  templateEngine: TemplateEngine,
  templateKey: string,
): LoadedTemplate => {
  if (!Object.prototype.hasOwnProperty.call(templateEngine.templates, templateKey)) {
    throw new Error(`template source missing for key: ${templateKey}`);
  }
  const source = templateEngine.templates[templateKey];
  const parsed = templateEngine.engine.parseFileSync(templateKey);
  return {
    name: templateKey,
    filePath: templateKey,
    source,
    parsed,
  };
};

export const renderTemplate = (
  templateEngine: TemplateEngine,
  template: LoadedTemplate,
  context: Record<string, unknown>,
): string => {
  const rendered = templateEngine.engine.renderSync(template.parsed, context) as unknown;
  if (typeof rendered === 'string') return rendered;
  return String(rendered);
};
