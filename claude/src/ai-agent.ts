/**
 * AI Agent Core - Production Implementation
 * Following IMPLEMENTATION.md specification exactly
 */

import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { dynamicTool, jsonSchema } from '@ai-sdk/provider-utils';
import { streamText } from 'ai';

import type { 
  AIAgentCallbacks,
  AIAgentOptions, 
  AIAgentRunOptions, 
  Configuration,
  ConversationMessage,
  TokenUsage
} from './types.js';

import { AccountingManager } from './accounting.js';
import { loadConfiguration, validateMCPServers, validatePrompts, validateProviders } from './config.js';
import { MCPClientManager } from './mcp-client.js';

const UNKNOWN_ERROR_MESSAGE = 'Unknown error';

export class AIAgent {
  private config: Configuration;
  private mcpManager: MCPClientManager;
  private accounting: AccountingManager;
  private callbacks?: AIAgentCallbacks;
  private options: {
    configPath: string;
    llmTimeout: number;
    toolTimeout: number;
    maxParallelTools?: number;
    maxConcurrentTools?: number;
    parallelToolCalls: boolean;
    temperature: number;
    topP: number;
  };

  constructor(options: AIAgentOptions = {}) {
    // Load and validate configuration
    this.config = loadConfiguration(options.configPath);
    
    // Set up callbacks
    this.callbacks = options.callbacks;
    
    // Merge options with config defaults per spec
    const defaults = this.config.defaults ?? {};
    this.options = {
      configPath: options.configPath ?? '',
      llmTimeout: options.llmTimeout ?? defaults.llmTimeout ?? 30000,
      toolTimeout: options.toolTimeout ?? defaults.toolTimeout ?? 10000,
      maxParallelTools: options.maxParallelTools,
      maxConcurrentTools: options.maxConcurrentTools,
      parallelToolCalls: options.parallelToolCalls ?? defaults.parallelToolCalls ?? true,
      temperature: options.temperature ?? defaults.temperature ?? 0.7,
      topP: options.topP ?? defaults.topP ?? 1.0,
    };

    // Initialize MCP manager and accounting
    this.mcpManager = new MCPClientManager(this.options.toolTimeout);
    this.accounting = new AccountingManager(this.callbacks, this.config.accounting?.file);
  }

  /**
   * Run the AI agent with streaming output per IMPLEMENTATION.md
   */
  async run(runOptions: AIAgentRunOptions): Promise<{
    conversation: ConversationMessage[];
    success: boolean;
    error?: string;
  }> {
    try {
      // Validate configuration per spec
      validateProviders(this.config, runOptions.providers);
      validateMCPServers(this.config, runOptions.tools);
      validatePrompts(runOptions.systemPrompt, runOptions.userPrompt);

      // Initialize MCP clients and discover tools/prompts per spec
      await this.mcpManager.initializeServers(this.config.mcpServers);

      // Build AI SDK tools from MCP tools per spec line 61-64
      const aiTools = this.buildAISDKTools();

      // Enhance system prompt with instructions per spec line 99-103
      const enhancedSystemPrompt = this.enhanceSystemPrompt(
        runOptions.systemPrompt, 
        this.mcpManager.buildInstructions()
      );

      // Prepare messages for AI SDK per spec
      const messages = this.prepareMessages(runOptions, enhancedSystemPrompt);

      // Execute with provider fallback using streamText per spec line 79
      const result = await this.processWithProviderFallback(
        messages, 
        aiTools,
        runOptions.providers, 
        runOptions.models
      );
      
      if (!result.success) {
        return { conversation: [], success: false, error: result.error };
      }

      return { 
        conversation: result.conversation ?? [], 
        success: true 
      };

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : UNKNOWN_ERROR_MESSAGE;
      this.log('error', `AI Agent failed: ${errorMessage}`);
      return { conversation: [], success: false, error: errorMessage };
    } finally {
      // Clean up MCP connections per spec
      await this.mcpManager.cleanup();
    }
  }

  /**
   * Build AI SDK tools from MCP tools per spec line 61-64
   */
  private buildAISDKTools(): Record<string, unknown> {
    const aiTools: Record<string, unknown> = {};
    const mcpTools = this.mcpManager.getTools();

    mcpTools.forEach((toolInfo, toolName) => {
      aiTools[toolName] = dynamicTool({
        description: toolInfo.description ?? toolInfo.title ?? `Tool: ${toolName}`,
        inputSchema: jsonSchema({
          ...toolInfo.inputSchema,
          properties: toolInfo.inputSchema.properties ?? {},
          additionalProperties: false,
        }),
        execute: async (args: Record<string, unknown>): Promise<string> => {
          const startTime = Date.now();
          
          try {
            // Call MCP tool with timeout and error handling per spec
            const result = await this.mcpManager.callTool(toolName, args);
            const latency = Date.now() - startTime;

            // Extract text content per spec line 142
            const textContent = result.content
              .map(c => c.type === 'text' ? c.text ?? '' : `[${c.type}]`)
              .join('\n');

            // Log tool usage per spec line 95
            this.accounting.logToolUsage(
              toolName,
              toolInfo.serverName,
              'unknown', // command not available in this context
              {
                latency,
                charactersIn: JSON.stringify(args).length,
                charactersOut: textContent.length,
                success: result.isError !== true,
              },
              result.isError === true ? 'Tool execution error' : undefined
            );

            // Per spec line 114: failed tools continue conversation for self-correction
            if (result.isError === true) {
              return `Tool execution failed: ${textContent}`;
            }

            return textContent;

          } catch (error) {
            const latency = Date.now() - startTime;
            const errorMessage = error instanceof Error ? error.message : UNKNOWN_ERROR_MESSAGE;
            
            // Log failed tool execution
            this.accounting.logToolUsage(
              toolName,
              toolInfo.serverName,
              'unknown',
              {
                latency,
                charactersIn: JSON.stringify(args).length,
                charactersOut: 0,
                success: false,
              },
              errorMessage
            );

            // Per spec: return error as text for LLM to handle
            return `Tool execution failed: ${errorMessage}`;
          }
        }
      });
    });

    this.log('info', `Built ${String(Object.keys(aiTools).length)} AI SDK tools from MCP servers`);
    return aiTools;
  }

  /**
   * Process with model-first fallback using streamText per spec line 79, 107
   */
  private async processWithProviderFallback(
    messages: unknown[],
    tools: Record<string, unknown>,
    providers: string[],
    models: string[]
  ): Promise<{
    success: boolean;
    conversation?: ConversationMessage[];
    error?: string;
  }> {
    // Model-first fallback per spec line 107 - generate all combinations
    const combinations = models.flatMap(model => 
      providers.map(provider => ({ model, provider }))
    );

    return await this.tryProviderCombinations(combinations, messages, tools);
  }

  /**
   * Try provider combinations until one succeeds
   */
  private async tryProviderCombinations(
    combinations: { model: string; provider: string }[],
    messages: unknown[],
    tools: Record<string, unknown>
  ): Promise<{
    success: boolean;
    conversation?: ConversationMessage[];
    error?: string;
  }> {
    return await this.tryNextCombination(combinations, 0, messages, tools);
  }

  /**
   * Recursively try provider combinations
   */
  private async tryNextCombination(
    combinations: { model: string; provider: string }[],
    index: number,
    messages: unknown[],
    tools: Record<string, unknown>
  ): Promise<{
    success: boolean;
    conversation?: ConversationMessage[];
    error?: string;
  }> {
    if (index >= combinations.length) {
      return { success: false, error: 'All providers and models failed' };
    }

    const { model, provider } = combinations[index] ?? { model: '', provider: '' };
        try {
          this.log('info', `Trying ${provider}/${model}`);
          
          const startTime = Date.now();
          const llmProvider = this.getLLMProvider(provider);
          
          // Use streamText per spec line 79
          const result = streamText({
            model: llmProvider(model),
            messages,
            tools,
            temperature: this.options.temperature,
            topP: this.options.topP,
            // OpenAI parallel tool calls per spec line 87
            ...(provider === 'openai' || provider === 'openrouter' ? {
              providerOptions: {
                openai: {
                  parallelToolCalls: this.options.parallelToolCalls
                }
              }
            } : {}),
            abortSignal: AbortSignal.timeout(this.options.llmTimeout),
            onStepFinish: (step) => {
              // Stream text-delta to stdout per spec line 88-89
              if (step.text) {
                this.output(step.text);
              }
            },
          });

          // Stream the text output in real-time per spec  
          await this.streamTextChunks(result.textStream);

          const endTime = Date.now();
          const latency = endTime - startTime;

          // Get final usage and response per spec line 90-91
          const [finalUsage, finalResponse] = await Promise.all([
            result.usage,
            result.response
          ]);

          // Log LLM usage per spec line 94
          this.accounting.logLLMUsage(
            provider,
            model,
            this.normalizeTokenUsage(finalUsage),
            latency,
            'ok'
          );

          // Build conversation from response per spec
          const conversation = this.buildConversationFromResponse(
            finalResponse, 
            provider, 
            model, 
            finalUsage
          );

          return {
            success: true,
            conversation,
          };

        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : UNKNOWN_ERROR_MESSAGE;
          this.log('warn', `${provider}/${model} failed: ${errorMessage}`);
          
          // Log failed LLM usage per spec
          this.accounting.logLLMUsage(
            provider,
            model,
            { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
            0,
            'failed',
            errorMessage
          );
          
          // Try next provider/model per spec line 108
          return await this.tryNextCombination(combinations, index + 1, messages, tools);
        }
  }

  /**
   * Prepare messages for AI SDK per spec
   */
  private prepareMessages(runOptions: AIAgentRunOptions, enhancedSystemPrompt: string): unknown[] {
    const messages: unknown[] = [];
    
    // Handle conversation history per spec
    if (runOptions.conversationHistory !== undefined && runOptions.conversationHistory.length > 0) {
      // Convert existing messages, replacing system prompt
      runOptions.conversationHistory.forEach(msg => {
        if (msg.role === 'system') {
          // Replace with enhanced system prompt
          messages.push({
            role: 'system',
            content: enhancedSystemPrompt,
          });
        } else {
          // Convert other messages to AI SDK format
          const aiMessage: Record<string, unknown> = {
            role: msg.role,
            content: msg.content,
          };
          
          // Convert tool calls if present
          if (msg.toolCalls !== undefined) {
            aiMessage.toolInvocations = msg.toolCalls.map(call => ({
              toolCallId: call.id,
              toolName: call.name,
              args: call.parameters,
            }));
          }
          
          if (msg.toolCallId !== undefined) {
            aiMessage.toolInvocationId = msg.toolCallId;
          }
          
          messages.push(aiMessage);
        }
      });
      
      // Add system prompt if not found in history
      if (!runOptions.conversationHistory.some(m => m.role === 'system')) {
        messages.unshift({
          role: 'system',
          content: enhancedSystemPrompt,
        });
      }
    } else {
      // New conversation - add system prompt
      messages.push({
        role: 'system', 
        content: enhancedSystemPrompt,
      });
    }
    
    // Add new user prompt
    messages.push({
      role: 'user',
      content: runOptions.userPrompt,
    });
    
    return messages;
  }

  /**
   * Build conversation from AI SDK response
   */
  private buildConversationFromResponse(
    response: Record<string, unknown>, 
    provider: string, 
    model: string, 
    usage: Record<string, unknown>
  ): ConversationMessage[] {
    const conversation: ConversationMessage[] = [];
    
    // Process all messages from the response
    const messages = response.messages ?? [];
    if (Array.isArray(messages)) {
      const safeStringify = (value: unknown): string => {
        if (typeof value === 'string') return value;
        if (typeof value === 'number' || typeof value === 'boolean') return String(value);
        return '';
      };

      messages.forEach((message: Record<string, unknown>) => {
        const conversationMessage: ConversationMessage = {
          role: safeStringify(message.role),
          content: safeStringify(message.content),
          metadata: {
            provider,
            model,
            tokens: this.normalizeTokenUsage(usage),
            timestamp: Date.now(),
          },
        };
        
        // Add tool calls if present
        if (message.toolInvocations !== undefined && Array.isArray(message.toolInvocations)) {
          conversationMessage.toolCalls = message.toolInvocations.map((inv: Record<string, unknown>) => ({
            id: safeStringify(inv.toolCallId),
            name: safeStringify(inv.toolName),
            parameters: inv.args as Record<string, unknown>,
          }));
        }
        
        if (message.toolInvocationId !== undefined) {
          conversationMessage.toolCallId = safeStringify(message.toolInvocationId);
        }
        
        conversation.push(conversationMessage);
      });
    }
    
    return conversation;
  }

  /**
   * Normalize token usage across different providers per spec line 90
   */
  private normalizeTokenUsage(usage?: Record<string, unknown> | null): TokenUsage {
    if (usage === undefined || usage === null) {
      return { inputTokens: 0, outputTokens: 0, totalTokens: 0 };
    }

    const safeNumber = (value: unknown): number => {
      const num = Number(value);
      return Number.isNaN(num) ? 0 : num;
    };

    return {
      // Handle different provider formats per spec line 90
      inputTokens: safeNumber(usage.inputTokens) || safeNumber(usage.promptTokens) || 0,
      outputTokens: safeNumber(usage.outputTokens) || safeNumber(usage.completionTokens) || 0,
      cachedTokens: safeNumber(usage.cachedTokens) || safeNumber(usage.cachedInputTokens) || 0,
      totalTokens: safeNumber(usage.totalTokens) || 
        (safeNumber(usage.inputTokens) || safeNumber(usage.promptTokens) || 0) + 
        (safeNumber(usage.outputTokens) || safeNumber(usage.completionTokens) || 0),
    };
  }

  /**
   * Get LLM provider instance per spec line 72-77
   */
  private getLLMProvider(providerName: string): (model: string) => Record<string, unknown> {
    const providerConfig = this.config.providers[providerName];
    
    switch (providerName) {
      case 'openai':
        const openaiProvider = createOpenAI({
          apiKey: providerConfig.apiKey,
          baseURL: providerConfig.baseUrl,
          headers: {
            'Accept': 'application/json', // Security per spec line 118
            ...providerConfig.headers
          },
        });
        return (model: string) => openaiProvider(model);
      
      case 'anthropic':
        const anthropicProvider = createAnthropic({
          apiKey: providerConfig.apiKey,
          baseURL: providerConfig.baseUrl,
          headers: providerConfig.headers,
        });
        return (model: string) => anthropicProvider(model);
      
      case 'google':
        const googleProvider = createGoogleGenerativeAI({
          apiKey: providerConfig.apiKey,
          baseURL: providerConfig.baseUrl,
          headers: providerConfig.headers,
        });
        return (model: string) => googleProvider(model);

      case 'openrouter':
        const openrouterProvider = createOpenAI({
          apiKey: providerConfig.apiKey,
          baseURL: providerConfig.baseUrl ?? 'https://openrouter.ai/api/v1',
          headers: {
            'Accept': 'application/json',
            // Attribution headers per spec line 119
            'HTTP-Referer': 'https://ai-agent.local',
            'X-OpenRouter-Title': 'AI Agent',
            'User-Agent': 'AI-Agent/1.0.0',
            ...providerConfig.headers
          },
          name: 'openrouter', // Per spec line 76
        });
        return (model: string) => openrouterProvider.chat(model); // Force Chat Completions per spec
      
      case 'ollama':
        const ollamaProvider = createOpenAI({
          apiKey: providerConfig.apiKey ?? 'ollama',
          baseURL: providerConfig.baseUrl ?? 'http://localhost:11434/v1',
          headers: providerConfig.headers,
        });
        return (model: string) => ollamaProvider(model);
      
      default:
        throw new Error(`Unsupported provider: ${providerName}`);
    }
  }

  /**
   * Enhance system prompt with MCP instructions per spec line 99-103
   */
  private enhanceSystemPrompt(systemPrompt: string, mcpInstructions: string): string {
    if (mcpInstructions.trim() === '') {
      return systemPrompt;
    }

    // Use exact format from spec line 100
    return `${systemPrompt}\n\n## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`;
  }
  
  /**
   * Log message using callback or stderr per spec line 56
   */
  private log(level: 'debug' | 'info' | 'warn' | 'error', message: string): void {
    if (this.callbacks?.onLog !== undefined) {
      this.callbacks.onLog(level, message);
    } else {
      console.error(`[${level.toUpperCase()}] ${message}`);
    }
  }

  /**
   * Output text using callback or stdout per spec line 88-89
   */
  private output(text: string): void {
    if (this.callbacks?.onOutput !== undefined) {
      this.callbacks.onOutput(text);
    } else {
      process.stdout.write(text);
    }
  }

  /**
   * Stream text chunks functionally
   */
  private async streamTextChunks(textStream: AsyncIterable<string>): Promise<void> {
    const iterator = textStream[Symbol.asyncIterator]();
    await this.processNextChunk(iterator);
  }

  /**
   * Process next chunk recursively
   */
  private async processNextChunk(iterator: AsyncIterator<string>): Promise<void> {
    const result = await iterator.next();
    if (result.done === true) {
      return;
    }
    this.output(result.value);
    await this.processNextChunk(iterator);
  }
}