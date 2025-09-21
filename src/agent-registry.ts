import path from 'node:path';

import type { LoadAgentOptions, LoadedAgent } from './agent-loader.js';
import type { AIAgentSession } from './ai-agent.js';
import type { OutputFormatId } from './formats.js';
import type { AIAgentCallbacks, ConversationMessage } from './types.js';

import { loadAgent, LoadedAgentCache } from './agent-loader.js';
import { describeFormat } from './formats.js';
import { applyFormat, buildPromptVars, expandVars } from './prompt-builder.js';

const OUTPUT_FORMAT_VALUES: readonly OutputFormatId[] = ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'] as const;

const cloneSchema = (schema: Record<string, unknown>): Record<string, unknown> => JSON.parse(JSON.stringify(schema)) as Record<string, unknown>;
const cloneSchemaOptional = (schema?: Record<string, unknown>): Record<string, unknown> | undefined => (schema === undefined ? undefined : cloneSchema(schema));
const isOutputFormatId = (value: string): value is OutputFormatId => OUTPUT_FORMAT_VALUES.includes(value as OutputFormatId);

export interface AgentMetadata {
  id: string;
  promptPath: string;
  description?: string;
  usage?: string;
  toolName?: string;
  expectedOutput?: LoadedAgent['expectedOutput'];
  input: { format: 'json' | 'text'; schema: Record<string, unknown> };
  outputSchema?: Record<string, unknown>;
}

export interface SpawnSessionArgs {
  agentId: string;
  payload?: Record<string, unknown>;
  userPrompt?: string;
  format?: OutputFormatId;
  history?: ConversationMessage[];
  callbacks?: AIAgentCallbacks;
  renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent';
  abortSignal?: AbortSignal;
  stopRef?: { stopping: boolean };
  initialTitle?: string;
  ancestors?: string[];
}

export class AgentRegistry {
  private readonly cache = new LoadedAgentCache();
  private readonly agents = new Map<string, LoadedAgent>();
  private readonly aliasToId = new Map<string, string>();

  public constructor(agentPaths: string[], loadOptions?: LoadAgentOptions) {
    const queue = agentPaths.map((maybePath) => path.resolve(maybePath));
    this.loadAgentsRecursive(queue, loadOptions, new Set<string>());
  }

  public list(): AgentMetadata[] {
    return Array.from(this.agents.values()).map((agent) => ({
      id: agent.id,
      promptPath: agent.promptPath,
      description: agent.description,
      usage: agent.usage,
      toolName: agent.toolName,
      expectedOutput: agent.expectedOutput !== undefined
        ? { ...agent.expectedOutput, schema: cloneSchemaOptional(agent.expectedOutput.schema) }
        : undefined,
      input: { format: agent.input.format, schema: cloneSchema(agent.input.schema) },
      outputSchema: cloneSchemaOptional(agent.outputSchema),
    }));
  }

  public getMetadata(agentId: string): AgentMetadata | undefined {
    const actualId = this.resolveAgentId(agentId);
    const agent = actualId !== undefined ? this.agents.get(actualId) : undefined;
    if (agent === undefined) return undefined;
    return {
      id: agent.id,
      promptPath: agent.promptPath,
      description: agent.description,
      usage: agent.usage,
      toolName: agent.toolName,
      expectedOutput: agent.expectedOutput !== undefined
        ? { ...agent.expectedOutput, schema: cloneSchemaOptional(agent.expectedOutput.schema) }
        : undefined,
      input: { format: agent.input.format, schema: cloneSchema(agent.input.schema) },
      outputSchema: cloneSchemaOptional(agent.outputSchema),
    };
  }

  public has(agentId: string): boolean {
    return this.resolveAgentId(agentId) !== undefined;
  }

  public async spawnSession(args: SpawnSessionArgs): Promise<AIAgentSession> {
    const actualId = this.resolveAgentId(args.agentId);
    const agent = actualId !== undefined ? this.agents.get(actualId) : undefined;
    if (agent === undefined) {
      throw new Error(`Unknown agent '${args.agentId}'`);
    }
    const payload = args.payload ?? {};
    const payloadFormatRaw = typeof payload.format === 'string' ? payload.format : undefined;
    const format: OutputFormatId = args.format
      ?? (payloadFormatRaw !== undefined && isOutputFormatId(payloadFormatRaw) ? payloadFormatRaw : undefined)
      ?? 'markdown';
    const userPrompt = typeof args.userPrompt === 'string' && args.userPrompt.length > 0
      ? args.userPrompt
      : (() => {
          const maybePrompt = (payload as { prompt?: unknown }).prompt;
          if (typeof maybePrompt === 'string' && maybePrompt.length > 0) return maybePrompt;
          throw new Error(`spawnSession for '${args.agentId}' requires a user prompt (use args.userPrompt or payload.prompt)`);
        })();

    const systemPrompt = this.buildSystemPrompt(agent, format, payload);

    return await agent.createSession(systemPrompt, userPrompt, {
      history: args.history,
      callbacks: args.callbacks,
      renderTarget: args.renderTarget,
      outputFormat: format,
      abortSignal: args.abortSignal,
      stopRef: args.stopRef,
      initialTitle: args.initialTitle,
      ancestors: args.ancestors,
    });
  }

  private buildSystemPrompt(agent: LoadedAgent, format: OutputFormatId, payload: Record<string, unknown>): string {
    const templ = applyFormat(agent.systemTemplate, describeFormat(format));
    const vars = { ...buildPromptVars() } as Record<string, string>;
    Object.entries(payload).forEach(([key, value]) => {
      if (typeof value !== 'string') return;
      vars[key] = value;
      const upper = key.toUpperCase();
      if (!(upper in vars)) vars[upper] = value;
    });
    return expandVars(templ, vars);
  }

  public resolveAgentId(alias: string): string | undefined {
    if (this.agents.has(alias)) return alias;
    return this.aliasToId.get(alias);
  }

  private registerAliases(agent: LoadedAgent, originalPath: string): void {
    const add = (alias: string) => {
      const trimmed = alias.trim();
      if (trimmed.length === 0) return;
      if (this.aliasToId.has(trimmed)) return;
      this.aliasToId.set(trimmed, agent.id);
    };

    add(agent.id);
    add(agent.promptPath);
    add(originalPath);
    add(path.basename(originalPath));
    const withoutExt = path.basename(originalPath).replace(/\.[^.]+$/, '');
    add(withoutExt);
    if (agent.toolName !== undefined) add(agent.toolName);
  }

  private loadAgentsRecursive(queue: string[], loadOptions: LoadAgentOptions | undefined, visited: Set<string>): void {
    if (queue.length === 0) return;
    const [candidate, ...rest] = queue;
    const resolvedPath = path.resolve(candidate);
    if (visited.has(resolvedPath)) {
      this.loadAgentsRecursive(rest, loadOptions, visited);
      return;
    }
    visited.add(resolvedPath);
    const sanitizedOptions: LoadAgentOptions | undefined = loadOptions === undefined ? undefined : { ...loadOptions };
    const loaded = loadAgent(resolvedPath, this.cache, sanitizedOptions);
    if (!this.agents.has(loaded.id)) {
      this.agents.set(loaded.id, loaded);
    }
    this.registerAliases(loaded, resolvedPath);
    const subAgents = Array.isArray(loaded.subAgentPaths) ? loaded.subAgentPaths : [];
    const nextQueue = rest.concat(subAgents);
    this.loadAgentsRecursive(nextQueue, loadOptions, visited);
  }
}
