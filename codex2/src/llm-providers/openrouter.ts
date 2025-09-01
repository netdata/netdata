import { createOpenRouter } from '@openrouter/ai-sdk-provider';

import type { ProviderConfig } from '../types.js';
import type { LanguageModel } from 'ai';

export function makeOpenRouterProvider(cfg: ProviderConfig, tracedFetch: typeof fetch): (model: string) => LanguageModel {
  const prov = createOpenRouter({
    apiKey: cfg.apiKey,
    baseURL: cfg.baseUrl,
    fetch: tracedFetch,
    headers: {
      'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
      'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-codex2',
      'User-Agent': 'ai-agent-codex2/1.0',
      ...(cfg.headers ?? {}),
    },
  });
  return (m: string) => prov(m);
}
