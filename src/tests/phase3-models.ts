export interface Phase3ModelConfig {
  readonly label: string;
  readonly provider: string;
  readonly modelId: string;
  readonly tier: 1 | 2 | 3;
}

// Phase 3 uses free nova models only (local inference)
// All paid providers (openai, anthropic, google, openrouter) are disabled
export const phase3ModelConfigs: readonly Phase3ModelConfig[] = [
  // Tier 1: Fast, free models for basic validation (nova GPUs)
  {
    label: "nova/minimax-m2.1",
    provider: "nova",
    modelId: "minimax-m2.1",
    tier: 1,
  },
  {
    label: "nova/glm-4.5-air",
    provider: "nova",
    modelId: "glm-4.5-air",
    tier: 1,
  },
  {
    label: "nova/glm-4.6",
    provider: "nova",
    modelId: "glm-4.6",
    tier: 1,
  },
  {
    label: "nova/glm-4.7",
    provider: "nova",
    modelId: "glm-4.7",
    tier: 1,
  },
] as const;
