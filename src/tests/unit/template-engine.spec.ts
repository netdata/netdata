import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { collectTemplateSources, createTemplateEngine, loadTemplate, renderTemplate, templateKeyFromFilePath } from '../../prompts/template-engine.js';

const FIXTURES_DIR = fileURLToPath(new URL('../fixtures/liquid/', import.meta.url));

const rootKey = (): string => templateKeyFromFilePath(FIXTURES_DIR, join(FIXTURES_DIR, 'root.md'));
const childKey = (): string => templateKeyFromFilePath(FIXTURES_DIR, join(FIXTURES_DIR, 'partials', 'child.md'));

describe('Liquid template engine', () => {
  it('collects includes with whitespace control tags', () => {
    const templates = collectTemplateSources(FIXTURES_DIR, ['root.md']);
    expect(templates[rootKey()]).toBeDefined();
    expect(templates[childKey()]).toBeDefined();
  });

  it('rejects dynamic include paths', () => {
    expect(() => collectTemplateSources(FIXTURES_DIR, ['dynamic.md'])).toThrowError(/static quoted path/i);
  });

  it('fails when includes are missing', () => {
    expect(() => collectTemplateSources(FIXTURES_DIR, ['missing.md'])).toThrowError(/include file not found/i);
  });

  it('blocks .env includes', () => {
    expect(() => collectTemplateSources(FIXTURES_DIR, ['env.md'])).toThrowError(/including this file is forbidden/i);
  });

  it('throws on missing variables in strict mode', () => {
    const engine = createTemplateEngine({ 'root.md': 'Hello {{ name }}' });
    const template = loadTemplate(engine, 'root.md');
    expect(() => renderTemplate(engine, template, {})).toThrowError();
  });

  it('renders includes with provided variables', () => {
    const templates = collectTemplateSources(FIXTURES_DIR, ['root.md']);
    const engine = createTemplateEngine(templates);
    const template = loadTemplate(engine, rootKey());
    const rendered = renderTemplate(engine, template, { name: 'World' });
    expect(rendered).toContain('Child World');
  });
});
