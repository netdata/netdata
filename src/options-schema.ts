import { z } from 'zod';

import { OPTIONS_REGISTRY } from './options-registry.js';

// Define the list of effective runtime option keys we validate post-resolution
const EFFECTIVE_KEYS = [
  'temperature',
  'topP',
  'topK',
  'maxOutputTokens',
  'repeatPenalty',
  'llmTimeout',
  'toolTimeout',
  'maxRetries',
  'maxToolTurns',
  'maxToolCallsPerTurn',
  'toolResponseMaxBytes',
  'stream',
  'traceLLM',
  'traceMCP',
  'traceSlack',
  'traceSdk',
  'verbose',
  'mcpInitConcurrency',
  'reasoning',
  'caching',
] as const;

type EffectiveKey = typeof EFFECTIVE_KEYS[number];

export function buildEffectiveOptionsSchema(): z.ZodObject<Record<EffectiveKey, z.ZodType>> {
  const defByKey = OPTIONS_REGISTRY.reduce<Record<string, typeof OPTIONS_REGISTRY[number]>>((acc, d) => {
    acc[d.key] = d;
    return acc;
  }, {});

  const must = <T>(v: T | undefined, msg: string): T => {
    if (v === undefined) throw new Error(msg);
    return v;
  };

  // Numeric fields that allow null (meaning "do not send to provider")
  const nullableNumericKeys = new Set(['temperature', 'topP', 'topK', 'repeatPenalty']);

  const shapeEntries = EFFECTIVE_KEYS.map((key) => {
    const def = must(defByKey[key], `Option registry missing definition for key '${key}'`);
    if (def.type === 'boolean') {
      return [key, z.boolean()] as const;
    }
    if (def.type === 'number') {
      let s = z.number();
      if (def.numeric?.integer === true) s = s.int();
      if (typeof def.numeric?.min === 'number') s = s.min(def.numeric.min);
      if (typeof def.numeric?.max === 'number') s = s.max(def.numeric.max);
      // mcpInitConcurrency is optional in effective options
      if (key === 'mcpInitConcurrency') {
        return [key, s.optional()] as const;
      }
      // Allow null for nullable numeric params (temperature, topP, topK, repeatPenalty)
      if (nullableNumericKeys.has(key)) {
        return [key, z.union([s, z.null()])] as const;
      }
      return [key, s] as const;
    }
    if (def.type === 'string') {
      return [key, z.string().optional()] as const;
    }
    // Non-numeric/string options are not expected in effective shape; fallback to any
    return [key, z.any()] as const;
  });

  const shape = shapeEntries.reduce<Record<string, z.ZodType>>((acc, [k, v]) => {
    acc[k] = v;
    return acc;
  }, {});

  return z.object(shape as Record<EffectiveKey, z.ZodType>);
}
