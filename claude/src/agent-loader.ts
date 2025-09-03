import fs from 'node:fs';

import * as yaml from 'js-yaml';

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from './types.js';

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers } from './config-resolver.js';

export interface LoadedAgent {
  id: string; // canonical path
  promptPath: string;
  description?: string;
  usage?: string;
  expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> };
  config: Configuration;
  subTools: { name: string; description?: string }[]; // placeholder for future
  run: (systemPrompt: string, userPrompt: string, history?: ConversationMessage[]) => Promise<AIAgentResult>;
}

export class AgentRegistry {
  private readonly cache = new Map<string, LoadedAgent>();

  get(key: string): LoadedAgent | undefined { return this.cache.get(key); }
  set(key: string, val: LoadedAgent): void { this.cache.set(key, val); }
}

function canonical(p: string): string { return fs.realpathSync(p); }

function readFileText(p: string): string { return fs.readFileSync(p, 'utf-8'); }

function parseFrontmatter(src: string): { expectedOutput?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> }, options?: Record<string, unknown>, description?: string, usage?: string } | undefined {
  const m = /^---\n([\s\S]*?)\n---\n/.exec(src);
  if (m === null) return undefined;
  try {
    const rawUnknown: unknown = yaml.load(m[1]);
    if (typeof rawUnknown !== 'object' || rawUnknown === null) return undefined;
    const docObj = rawUnknown as { output?: { format?: string; schema?: unknown } } & Record<string, unknown>;
    let expectedOutput: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> } | undefined;
    if (docObj.output !== undefined && typeof docObj.output.format === 'string') {
      const f = docObj.output.format.toLowerCase();
      if (f === 'json') expectedOutput = { format: 'json', schema: (docObj.output.schema as Record<string, unknown> | undefined) };
      else if (f === 'markdown') expectedOutput = { format: 'markdown', schema: undefined };
      else if (f === 'text') expectedOutput = { format: 'text', schema: undefined };
    }
    const description = typeof (docObj as Record<string, unknown>).description === 'string' ? String((docObj as Record<string, unknown>).description) : undefined;
    const usage = typeof (docObj as Record<string, unknown>).usage === 'string' ? String((docObj as Record<string, unknown>).usage) : undefined;
    const options: Record<string, unknown> = {};
    return { expectedOutput, options, description, usage };
  } catch { return undefined; }
}

export function loadAgent(aiPath: string, registry?: AgentRegistry): LoadedAgent {
  const reg = registry ?? new AgentRegistry();
  const id = canonical(aiPath);
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const content = readFileText(aiPath);
  const fm = parseFrontmatter(content);

  // For now, let the caller provide targets/tools separately; we build config for none
  // Later we will parse fm.options for llms/tools and call resolver accordingly
  const layers = discoverLayers({ configPath: undefined });
  const config = buildUnifiedConfiguration({ providers: [], mcpServers: [] }, layers, { verbose: false });

  const loaded: LoadedAgent = {
    id,
    promptPath: id,
    description: fm?.description,
    usage: fm?.usage,
    expectedOutput: fm?.expectedOutput,
    config,
    subTools: [],
    run: async (systemPrompt: string, userPrompt: string, history?: ConversationMessage[]): Promise<AIAgentResult> => {
      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets: [],
        tools: [],
        systemPrompt,
        userPrompt,
        conversationHistory: history,
        callbacks: {},
      };
      const session = Agent.create(sessionConfig);
      return await session.run();
    }
  };
  reg.set(id, loaded);
  return loaded;
}
