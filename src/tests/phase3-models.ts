export interface Phase3ModelConfig {
  readonly label: string;
  readonly provider: string;
  readonly modelId: string;
  readonly tier: 1 | 2 | 3;
}

// Phase 3 uses free nova models only (local inference)
// All paid providers (openai, anthropic, google, openrouter) are disabled
export const phase3ModelConfigs: readonly Phase3ModelConfig[] = [
  // Tier 1: Fast, free models for basic validation
  {
    label: "nova/minimax-m2.1",
    provider: "nova",
    modelId: "minimax-m2.1",
    tier: 1,
  },
] as const;
