import { createAnthropic } from '@ai-sdk/anthropic';

import type { ProviderConfig } from '../types.js';
import type { LanguageModel } from 'ai';

export function makeAnthropicProvider(cfg: ProviderConfig, tracedFetch: typeof fetch): (model: string) => LanguageModel {
  const prov = createAnthropic({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
  return (m: string) => prov(m);
}
