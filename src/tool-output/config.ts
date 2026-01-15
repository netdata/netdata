import os from 'node:os';
import path from 'node:path';

import type { ToolOutputConfig, ToolOutputConfigInput, ToolOutputTarget } from './types.js';

import { parsePairs } from '../frontmatter.js';

const DEFAULT_MAX_CHUNKS = 1;
const DEFAULT_OVERLAP_PERCENT = 10;
const DEFAULT_AVG_LINE_BYTES_THRESHOLD = 1000;

const normalizePositiveInt = (value: unknown, fallback: number): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  const intVal = Math.trunc(value);
  if (intVal <= 0) return fallback;
  return intVal;
};

const normalizePercent = (value: unknown, fallback: number): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  const intVal = Math.trunc(value);
  if (intVal < 0) return fallback;
  return Math.min(50, intVal);
};

const normalizeModels = (value: unknown): ToolOutputTarget[] | undefined => {
  if (value === undefined) return undefined;
  const parsed = parsePairs(value);
  if (parsed.length === 0) return undefined;
  return parsed;
};

export function resolveToolOutputConfig(args: {
  config?: ToolOutputConfigInput;
  overrides?: ToolOutputConfigInput;
  baseDir?: string;
}): ToolOutputConfig {
  const base = args.config ?? {};
  const overrides = args.overrides ?? {};
  const enabled = overrides.enabled ?? base.enabled ?? true;
  const storeDirRaw = overrides.storeDir ?? base.storeDir;
  const storeDir = storeDirRaw !== undefined && storeDirRaw.length > 0
    ? path.resolve(args.baseDir ?? process.cwd(), storeDirRaw)
    : os.tmpdir();
  const maxChunks = normalizePositiveInt(overrides.maxChunks ?? base.maxChunks, DEFAULT_MAX_CHUNKS);
  const overlapPercent = normalizePercent(overrides.overlapPercent ?? base.overlapPercent, DEFAULT_OVERLAP_PERCENT);
  const avgLineBytesThreshold = normalizePositiveInt(overrides.avgLineBytesThreshold ?? base.avgLineBytesThreshold, DEFAULT_AVG_LINE_BYTES_THRESHOLD);
  const models = normalizeModels(overrides.models ?? base.models);
  return {
    enabled,
    storeDir,
    maxChunks,
    overlapPercent,
    avgLineBytesThreshold,
    models,
  };
}

export function resolveToolOutputTargets(
  config: ToolOutputConfig,
  sourceTarget: ToolOutputTarget | undefined,
  sessionTargets: ToolOutputTarget[]
): ToolOutputTarget[] {
  if (Array.isArray(config.models) && config.models.length > 0) {
    return config.models;
  }
  if (sourceTarget !== undefined) return [sourceTarget];
  return sessionTargets;
}
