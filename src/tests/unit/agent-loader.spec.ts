import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { loadAgentFromContent } from '../../agent-loader.js';

const FIXTURES_DIR = fileURLToPath(new URL('../fixtures/inline-prompt/', import.meta.url));
const MULTI_FIXTURES_DIR = fileURLToPath(new URL('../fixtures/inline-prompt-multi/', import.meta.url));

const buildPrompt = (): string => [
  '---',
  'models:',
  '  - test/deterministic-model',
  '---',
  "{% render 'docs/child.md' %}",
  '',
].join('\n');

describe('loadAgentFromContent', () => {
  it('resolves liquid includes relative to the inline baseDir', () => {
    const baseDir = path.relative(process.cwd(), FIXTURES_DIR);
    const configPath = path.join(FIXTURES_DIR, 'config.json');
    const loaded = loadAgentFromContent('inline-test', buildPrompt(), {
      baseDir,
      configPath,
    });
    expect(loaded.systemTemplate).toContain('Inline include content.');
  });

  it('resolves nested includes across folders relative to each referencing file', () => {
    const baseDir = path.relative(process.cwd(), path.join(MULTI_FIXTURES_DIR, 'A'));
    const configPath = path.join(MULTI_FIXTURES_DIR, 'config.json');
    const prompt = [
      '---',
      'models:',
      '  - test/deterministic-model',
      '---',
      "{% render 'B/ab1.md' %}",
      '',
    ].join('\n');
    const loaded = loadAgentFromContent('inline-multi-test', prompt, {
      baseDir,
      configPath,
    });
    const rendered = loaded.systemTemplate;
    expect(rendered).toContain('AB1 start.');
    expect(rendered).toContain('AB2 start.');
    expect(rendered).toContain('ABC1 start.');
    expect(rendered).toContain('AB3 final.');
    expect(rendered).toContain('ABC1 end.');
    expect(rendered).toContain('AB2 end.');
    expect(rendered).toContain('AB1 end.');
  });
});
