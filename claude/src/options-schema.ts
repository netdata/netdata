import { z } from 'zod';

import { OPTIONS_REGISTRY } from './options-registry.js';

// Define the list of effective runtime option keys we validate post-resolution
const EFFECTIVE_KEYS = [
  'temperature',
  'topP',
  'llmTimeout',
  'toolTimeout',
  'maxRetries',
  'maxToolTurns',
  'toolResponseMaxBytes',
  'stream',
  'parallelToolCalls',
  'maxConcurrentTools',
  'traceLLM',
  'traceMCP',
  'verbose',
  'mcpInitConcurrency',
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
      return [key, s] as const;
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
