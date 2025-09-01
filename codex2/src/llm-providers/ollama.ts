import { createOllama } from 'ollama-ai-provider-v2';

import type { ProviderConfig } from '../types.js';
import type { LanguageModel } from 'ai';

function normalizeBaseUrl(u?: string): string {
  const def = 'http://localhost:11434/api';
  if (typeof u !== 'string' || u.length === 0) return def;
  try {
    let v = u.replace(/\/$/, '');
    if (/\/v1\/?$/.test(v)) return v.replace(/\/v1\/?$/, '/api');
    if (/\/api\/?$/.test(v)) return v;
    return v + '/api';
  } catch { return def; }
}

export function makeOllamaProvider(cfg: ProviderConfig, tracedFetch: typeof fetch): (model: string) => LanguageModel {
  const prov = createOllama({ baseURL: normalizeBaseUrl(cfg.baseUrl), fetch: tracedFetch });
  return (m: string) => prov(m);
}
