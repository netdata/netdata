import { createGoogleGenerativeAI } from '@ai-sdk/google';

import type { ProviderConfig } from '../types.js';
import type { LanguageModel } from 'ai';

export function makeGoogleProvider(cfg: ProviderConfig, tracedFetch: typeof fetch): (model: string) => LanguageModel {
  const prov = createGoogleGenerativeAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
  return (m: string) => prov(m);
}
