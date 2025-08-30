import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { jsonSchema } from '@ai-sdk/provider-utils';
import { streamText } from 'ai';

import type { AccountingEntry, AIAgentCallbacks, AIAgentOptions, AIAgentRunOptions, Configuration, ConversationMessage } from './types.js';
import type { LanguageModel, ModelMessage, ToolSet } from 'ai';

import { loadConfiguration, validateMCPServers, validatePrompts, validateProviders } from './config.js';
import { MCPClientManager } from './mcp-client.js';

type ToolArgs = Record<string, unknown>;

export class AIAgent {
  private config: Configuration;
  private mcpClient: MCPClientManager;
  private callbacks: AIAgentCallbacks;
  private options: {
    configPath: string;
    llmTimeout: number;
    toolTimeout: number;
    temperature: number;
    topP: number;
    traceLLM?: boolean;
    traceMCP?: boolean;
    parallelToolCalls?: boolean;
    maxToolTurns: number;
  };

  constructor(options: AIAgentOptions = {}) {
    this.config = loadConfiguration(options.configPath);
    this.callbacks = options.callbacks ?? {};
    const defaults = this.config.defaults ?? {};
    this.options = {
      configPath: options.configPath ?? '',
      llmTimeout: options.llmTimeout ?? defaults.llmTimeout ?? 30000,
      toolTimeout: options.toolTimeout ?? defaults.toolTimeout ?? 10000,
      temperature: options.temperature ?? defaults.temperature ?? 0.7,
      topP: options.topP ?? defaults.topP ?? 1.0,
      traceLLM: options.traceLLM ?? false,
      traceMCP: options.traceMCP ?? false,
      parallelToolCalls: options.parallelToolCalls ?? defaults.parallelToolCalls ?? true,
      maxToolTurns: (options as { maxToolTurns?: number }).maxToolTurns ?? (defaults as { maxToolTurns?: number }).maxToolTurns ?? 30,
    };
    this.mcpClient = new MCPClientManager(this.options.toolTimeout);
  }

  private log(level: 'debug' | 'info' | 'warn' | 'error', message: string): void {
    this.callbacks.onLog?.(level, message);
  }

  private accounting(entry: AccountingEntry): void {
    this.callbacks.onAccounting?.(entry);
  }

  private output(text: string): void {
    this.callbacks.onOutput?.(text);
  }

  async run(options: AIAgentRunOptions): Promise<{ success: boolean; error?: string; conversation?: ConversationMessage[] }> {
    try {
      validateProviders(options.providers, this.config);
      validateMCPServers(options.tools, this.config);
      validatePrompts(options.systemPrompt, options.userPrompt);

      if (options.dryRun) {
        this.log('info', 'Dry run mode - skipping actual execution');
        return { success: true };
      }

      const conversation = [...(options.conversationHistory ?? [])];
      
      if (options.loadConversation) {
        this.log('info', `Loading conversation from: ${options.loadConversation}`);
      }

      const servers: Record<string, any> = {};
      for (const toolName of options.tools) {
        servers[toolName] = this.config.mcpServers[toolName]!;
      }
      
      await this.mcpClient.initializeServers(servers);

      const toolsMap = this.mcpClient.getTools();
      const availableTools = Array.from(toolsMap.values());
      const toolInstructions = this.mcpClient.buildInstructions();
      
      let systemPrompt = options.systemPrompt;
      if (toolInstructions.trim().length > 0) {
        systemPrompt += '\n\n## TOOLS\' INSTRUCTIONS\n\n';
        systemPrompt += toolInstructions;
      }

      const tools = this.createToolSet(availableTools);

      conversation.push({ role: 'system', content: systemPrompt });
      conversation.push({ role: 'user', content: options.userPrompt });

      const providers = this.createProviders(options.providers);
      
      for (const model of options.models) {
        for (const providerName of options.providers) {
          try {
            this.log('info', `Trying ${providerName}/${model}...`);
            
            const provider = providers[providerName];
            if (!provider) continue;

            const languageModel = provider(model);
            const messages = this.convertToMessages(conversation);

            const startTime = Date.now();
            
            let response = '';
            const result = await streamText({
              model: languageModel,
              messages,
              tools,
              temperature: this.options.temperature,
              topP: this.options.topP,
            });

            for await (const delta of result.textStream) {
              response += delta;
              this.output(delta);
            }

            const usage = await result.usage;

            this.accounting({
              type: 'llm',
              timestamp: Date.now(),
              status: 'ok',
              latency: Date.now() - startTime,
              provider: providerName,
              model,
              tokens: {
                inputTokens: usage.inputTokens ?? 0,
                outputTokens: usage.outputTokens ?? 0,
                totalTokens: usage.totalTokens ?? 0,
              }
            });

            conversation.push({
              role: 'assistant',
              content: response,
              metadata: {
                provider: providerName,
                model,
                tokens: {
                  inputTokens: usage.inputTokens ?? 0,
                  outputTokens: usage.outputTokens ?? 0,
                  totalTokens: usage.totalTokens ?? 0,
                },
                timestamp: Date.now(),
              }
            });

            if (options.saveConversation) {
              this.log('info', `Saving conversation to: ${options.saveConversation}`);
            }

            return { success: true, conversation };
          } catch (error) {
            this.log('error', `Failed with ${providerName}/${model}: ${error instanceof Error ? error.message : 'Unknown error'}`);
            this.accounting({
              type: 'llm',
              timestamp: Date.now(),
              status: 'failed',
              latency: Date.now() - Date.now(),
              provider: providerName,
              model,
              tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
              error: error instanceof Error ? error.message : 'Unknown error',
            });
          }
        }
      }

      return { success: false, error: 'All providers/models failed' };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      this.log('error', `Run failed: ${errorMessage}`);
      return { success: false, error: errorMessage };
    } finally {
      await this.mcpClient.cleanup();
    }
  }

  private createProviders(providerNames: string[]): Record<string, (model: string) => LanguageModel> {
    const providers: Record<string, (model: string) => LanguageModel> = {};

    for (const providerName of providerNames) {
      const cfg = this.config.providers[providerName];
      if (!cfg) continue;

      providers[providerName] = this.createProviderFactory(providerName, cfg);
    }

    return providers;
  }

  private createProviderFactory(providerName: string, cfg: any): (model: string) => LanguageModel {
    const tracedFetch = this.options.traceLLM ? this.createTracingFetch() : undefined;

    switch (providerName) {
      case 'openai': {
        const prov = createOpenAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      case 'anthropic': {
        const prov = createAnthropic({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      case 'google':
      case 'vertex': {
        const prov = createGoogleGenerativeAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      case 'openrouter': {
        const prov = createOpenAI({
          apiKey: cfg.apiKey,
          baseURL: cfg.baseUrl ?? 'https://openrouter.ai/api/v1',
          fetch: tracedFetch as typeof fetch,
          headers: {
            Accept: 'application/json',
            'HTTP-Referer': process.env['OPENROUTER_REFERER'] ?? 'https://ai-agent.local',
            'X-OpenRouter-Title': process.env['OPENROUTER_TITLE'] ?? 'ai-agent-claude',
            'User-Agent': 'ai-agent-claude/1.0',
          },
        });
        return (model: string) => prov(model);
      }
      case 'ollama': {
        const prov = createOpenAI({ apiKey: cfg.apiKey ?? 'ollama', baseURL: cfg.baseUrl ?? 'http://localhost:11434/v1', fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      default:
        throw new Error(`Unsupported provider: ${providerName}`);
    }
  }

  private createTracingFetch() {
    return async (url: string | URL, options?: RequestInit) => {
      this.log('debug', `[LLM] Request: ${JSON.stringify({ url, options }, null, 2)}`);
      const response = await fetch(url, options);
      this.log('debug', `[LLM] Response: ${response.status} ${response.statusText}`);
      return response;
    };
  }

  private convertToMessages(conversation: ConversationMessage[]): ModelMessage[] {
    return conversation.map(msg => ({
      role: msg.role as any,
      content: msg.content,
    }));
  }

  private createToolSet(availableTools: any[]): ToolSet {
    const tools: ToolSet = {};

    for (const tool of availableTools) {
      tools[tool.name] = {
        description: tool.description,
        inputSchema: jsonSchema(tool.inputSchema),
        execute: async (args: ToolArgs) => {
          const startTime = Date.now();
          try {
            const result = await this.mcpClient.callTool(tool.name, args);
            
            const resultText = result.content.map(c => c.text || '').join('\n');
            
            this.accounting({
              type: 'tool',
              timestamp: Date.now(),
              status: 'ok',
              latency: Date.now() - startTime,
              mcpServer: tool.serverName,
              command: tool.name,
              charactersIn: JSON.stringify(args).length,
              charactersOut: resultText.length,
            });

            return resultText;
          } catch (error) {
            const errorMessage = error instanceof Error ? error.message : 'Unknown error';
            
            this.accounting({
              type: 'tool',
              timestamp: Date.now(),
              status: 'failed',
              latency: Date.now() - startTime,
              mcpServer: tool.serverName,
              command: tool.name,
              charactersIn: JSON.stringify(args).length,
              charactersOut: 0,
              error: errorMessage,
            });

            throw error;
          }
        },
      };
    }

    return tools;
  }
}
