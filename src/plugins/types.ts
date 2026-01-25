import type { AIAgentSessionConfig, ConversationMessage, FinalReportPayload } from '../types.js';

/**
 * Requirements contributed by a final-report plugin.
 * These are injected into prompts and enforced at finalization time.
 */
export interface FinalReportPluginRequirements {
  schema: Record<string, unknown>;
  systemPromptInstructions: string;
  xmlNextSnippet: string;
  finalReportExampleSnippet: string;
}

// Requirements resolved for runtime usage (name attached, NONCE substituted)
export interface ResolvedFinalReportPluginRequirement extends FinalReportPluginRequirements {
  name: string;
}

export interface FinalReportPluginContext {
  sessionId: string;
  originId: string;
  agentId: string;
  agentPath: string;
  userRequest: string;
  messages: readonly ConversationMessage[];
  finalReport: FinalReportPayload;
  pluginData: Record<string, unknown>;
  fromCache: boolean;
  createSession: (config: AIAgentSessionConfig) => { run: () => Promise<unknown> };
}

export interface FinalReportPlugin {
  name: string;
  getRequirements: () => FinalReportPluginRequirements;
  onComplete: (context: FinalReportPluginContext) => Promise<void>;
}

// Session isolation: factories are loaded once, instances are created per session.
export type FinalReportPluginFactory = () => FinalReportPlugin;

/**
 * Plugin descriptors are prepared synchronously at agent-load time.
 * Actual module imports happen during session initialization in run().
 */
export interface PreparedFinalReportPluginDescriptor {
  rawPath: string;
  resolvedPath: string;
  fileHash: string;
}

export type FinalReportPluginMetaRecord = Record<string, Record<string, unknown>>;
