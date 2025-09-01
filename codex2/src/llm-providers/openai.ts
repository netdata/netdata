import { createOpenAI } from '@ai-sdk/openai';

import type { ProviderConfig } from '../types.js';
import type { LanguageModel } from 'ai';

export function makeOpenAIProvider(cfg: ProviderConfig, tracedFetch: typeof fetch): (mode: 'responses'|'chat') => (model: string) => LanguageModel {
  const prov = createOpenAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
  return (mode: 'responses'|'chat') => {
    if (mode === 'responses') return (m: string) => prov.responses(m);
    return (m: string) => prov.chat(m);
  };
}
