import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import type { FinalReportPluginFactory, PreparedFinalReportPluginDescriptor } from './types.js';

import { sha256Hex } from '../cache/hash.js';
import { stableStringify } from '../cache/stable-stringify.js';

const JS_EXTENSION = '.js';

const normalizePluginPath = (rawPath: string): string => {
  const trimmed = rawPath.trim();
  if (trimmed.length === 0) {
    throw new Error('Final report plugin path cannot be empty.');
  }
  if (path.isAbsolute(trimmed)) {
    throw new Error(`Final report plugin path must be relative to the agent file: '${trimmed}'.`);
  }
  if (path.extname(trimmed) !== JS_EXTENSION) {
    throw new Error(`Final report plugin must be a '${JS_EXTENSION}' file: '${trimmed}'.`);
  }
  return trimmed;
};

const readPluginFile = (resolvedPath: string): string => {
  try {
    const stat = fs.statSync(resolvedPath);
    if (!stat.isFile()) {
      throw new Error(`Final report plugin is not a file: '${resolvedPath}'.`);
    }
    return fs.readFileSync(resolvedPath, 'utf-8');
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`Failed to read final report plugin '${resolvedPath}': ${message}`);
  }
};

export interface PreparedPluginDescriptors {
  descriptors: PreparedFinalReportPluginDescriptor[];
  pluginHash: string | undefined;
}

/**
 * Synchronous preparation used by agent-loader.
 * Validates paths and computes a content hash without importing modules.
 */
export const prepareFinalReportPluginDescriptors = (agentDir: string, pluginPaths: string[]): PreparedPluginDescriptors => {
  if (pluginPaths.length === 0) {
    return { descriptors: [], pluginHash: undefined };
  }

  const seenResolved = new Set<string>();
  const descriptors = pluginPaths.map((rawPath) => {
    const normalized = normalizePluginPath(rawPath);
    const resolvedPath = path.resolve(agentDir, normalized);
    if (seenResolved.has(resolvedPath)) {
      throw new Error(`Duplicate final report plugin path resolved to '${resolvedPath}'.`);
    }
    seenResolved.add(resolvedPath);
    const contents = readPluginFile(resolvedPath);
    return {
      rawPath: normalized,
      resolvedPath,
      fileHash: sha256Hex(contents),
    } satisfies PreparedFinalReportPluginDescriptor;
  });

  const pluginHashPayload = descriptors.map((descriptor) => ({
    path: descriptor.rawPath,
    hash: descriptor.fileHash,
  }));

  return {
    descriptors,
    pluginHash: sha256Hex(stableStringify(pluginHashPayload)),
  };
};

const extractFactory = (moduleRecord: { default?: unknown }, descriptor: PreparedFinalReportPluginDescriptor): FinalReportPluginFactory => {
  const candidate = moduleRecord.default;
  if (typeof candidate !== 'function') {
    throw new Error(
      `Final report plugin '${descriptor.rawPath}' must default-export a factory function.`
    );
  }
  return candidate as FinalReportPluginFactory;
};

/**
 * Async module loading performed during session initialization in run().
 */
export const loadFinalReportPluginFactories = async (descriptors: PreparedFinalReportPluginDescriptor[]): Promise<FinalReportPluginFactory[]> => {
  if (descriptors.length === 0) {
    return [];
  }
  return await Promise.all(descriptors.map(async (descriptor) => {
    const url = pathToFileURL(descriptor.resolvedPath).href;
    try {
      const moduleRecord = (await import(url)) as { default?: unknown };
      return extractFactory(moduleRecord, descriptor);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      throw new Error(`Failed to import final report plugin '${descriptor.rawPath}': ${message}`);
    }
  }));
};
