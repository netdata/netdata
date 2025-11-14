export interface Phase2ModelConfig {
  readonly label: string;
  readonly provider: string;
  readonly modelId: string;
  readonly tier: 1 | 2 | 3;
}

export const phase2ModelConfigs: readonly Phase2ModelConfig[] = [
  {
    label: 'vllm/default-model',
    provider: 'vllm',
    modelId: 'default-model',
    tier: 1,
  },
  {
    label: 'ollama/gpt-oss:20b',
    provider: 'ollama',
    modelId: 'gpt-oss:20b',
    tier: 1,
  },
  {
    label: 'anthropic/claude-3-haiku-20240307',
    provider: 'anthropic',
    modelId: 'claude-3-haiku-20240307',
    tier: 2,
  },
  {
    label: 'openrouter/x-ai/grok-code-fast-1',
    provider: 'openrouter',
    modelId: 'x-ai/grok-code-fast-1',
    tier: 2,
  },
  {
    label: 'openai/gpt-5.1-mini',
    provider: 'openai',
    modelId: 'gpt-5-mini',
    tier: 2,
  },
  {
    label: 'google/gemini-2.5-flash',
    provider: 'google',
    modelId: 'gemini-2.5-flash',
    tier: 2,
  },
  {
    label: 'anthropic/claude-haiku-4-5',
    provider: 'anthropic',
    modelId: 'claude-haiku-4-5',
    tier: 3,
  },
  {
    label: 'openai/gpt-5.1',
    provider: 'openai',
    modelId: 'gpt-5',
    tier: 3,
  },
  {
    label: 'google/gemini-2.5-pro',
    provider: 'google',
    modelId: 'gemini-2.5-pro',
    tier: 3,
  },
  {
    label: 'anthropic/claude-sonnet-4-5',
    provider: 'anthropic',
    modelId: 'claude-sonnet-4-5',
    tier: 3,
  },
] as const;
