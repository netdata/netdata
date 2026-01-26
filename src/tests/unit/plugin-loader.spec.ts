import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import { sha256Hex } from '../../cache/hash.js';
import { stableStringify } from '../../cache/stable-stringify.js';
import { loadFinalReportPluginFactories, prepareFinalReportPluginDescriptors } from '../../plugins/loader.js';

const makeTempDir = (): string => fs.mkdtempSync(path.join(os.tmpdir(), 'plugin-loader-'));

const cleanupTempDir = (dir: string): void => {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // best-effort cleanup for unit tests
  }
};

const buildPluginSource = (name: string): string => `
export default function finalReportPluginFactory() {
  return {
    name: '${name}',
    getRequirements() {
      return {
        schema: { type: 'object' },
        systemPromptInstructions: 'Provide META JSON.',
        xmlNextSnippet: 'META required.',
        finalReportExampleSnippet: '<ai-agent-NONCE-META plugin="${name}">{}</ai-agent-NONCE-META>',
      };
    },
    async onComplete() {},
  };
}
`.trim();

const buildInvalidPluginSource = (): string => `
export default {
  name: 'invalid-export',
};
`.trim();

const PLUGIN_FILE_NAME = 'meta-plugin.js';
const UNIT_PLUGIN_NAME = 'unit-plugin';

describe('final report plugin loader', () => {
  it('returns empty descriptors and hash when no plugins are configured', () => {
    const prepared = prepareFinalReportPluginDescriptors(process.cwd(), []);
    expect(prepared.descriptors).toHaveLength(0);
    expect(prepared.pluginHash).toBeUndefined();
  });

  it('normalizes and hashes plugin descriptors', () => {
    const root = makeTempDir();
    const pluginPath = path.join(root, PLUGIN_FILE_NAME);
    const pluginSource = buildPluginSource(UNIT_PLUGIN_NAME);
    fs.writeFileSync(pluginPath, pluginSource, 'utf8');

    const prepared = prepareFinalReportPluginDescriptors(root, [PLUGIN_FILE_NAME]);
    expect(prepared.descriptors).toHaveLength(1);
    expect(prepared.descriptors[0]?.rawPath).toBe(PLUGIN_FILE_NAME);
    expect(prepared.descriptors[0]?.resolvedPath).toBe(path.resolve(root, PLUGIN_FILE_NAME));
    expect(prepared.descriptors[0]?.fileHash).toBe(sha256Hex(pluginSource));

    const expectedHashPayload = stableStringify([{ path: PLUGIN_FILE_NAME, hash: sha256Hex(pluginSource) }]);
    expect(prepared.pluginHash).toBe(sha256Hex(expectedHashPayload));

    cleanupTempDir(root);
  });

  it('rejects empty plugin paths', () => {
    expect(() => prepareFinalReportPluginDescriptors(process.cwd(), ['   ']))
      .toThrow('Final report plugin path cannot be empty.');
  });

  it('rejects absolute plugin paths', () => {
    expect(() => prepareFinalReportPluginDescriptors(process.cwd(), ['/abs/plugin.js']))
      .toThrow('Final report plugin path must be relative to the agent file');
  });

  it('rejects non-js plugin paths', () => {
    expect(() => prepareFinalReportPluginDescriptors(process.cwd(), ['plugin.ts']))
      .toThrow("Final report plugin must be a '.js' file");
  });

  it('rejects duplicate resolved plugin paths', () => {
    const root = makeTempDir();
    const pluginPath = path.join(root, PLUGIN_FILE_NAME);
    fs.writeFileSync(pluginPath, buildPluginSource('dup-plugin'), 'utf8');

    expect(() => prepareFinalReportPluginDescriptors(root, [PLUGIN_FILE_NAME, `./${PLUGIN_FILE_NAME}`]))
      .toThrow('Duplicate final report plugin path resolved to');

    cleanupTempDir(root);
  });

  it('loads plugin factories and validates default export', async () => {
    const root = makeTempDir();
    const pluginPath = path.join(root, PLUGIN_FILE_NAME);
    fs.writeFileSync(pluginPath, buildPluginSource(UNIT_PLUGIN_NAME), 'utf8');

    const prepared = prepareFinalReportPluginDescriptors(root, [PLUGIN_FILE_NAME]);
    const factories = await loadFinalReportPluginFactories(prepared.descriptors);
    expect(factories).toHaveLength(1);
    const instance = factories[0]();
    expect(instance.name).toBe(UNIT_PLUGIN_NAME);

    cleanupTempDir(root);
  });

  it('fails when plugin default export is not a factory function', async () => {
    const root = makeTempDir();
    const pluginName = 'invalid-plugin.js';
    const pluginPath = path.join(root, pluginName);
    fs.writeFileSync(pluginPath, buildInvalidPluginSource(), 'utf8');

    const prepared = prepareFinalReportPluginDescriptors(root, [pluginName]);
    await expect(loadFinalReportPluginFactories(prepared.descriptors))
      .rejects
      .toThrow(`Failed to import final report plugin '${pluginName}': Final report plugin '${pluginName}' must default-export a factory function.`);

    cleanupTempDir(root);
  });
});
