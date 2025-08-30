/**
 * MCP Client Layer - Production Implementation
 * Handles MCP server connections, tool discovery, and execution according to IMPLEMENTATION.md
 */
















import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';

import type { MCPServerConfig } from './types.js';
import type { JSONSchema7 } from 'json-schema';

import { createWebSocketTransport } from './websocket-transport.js';

export interface MCPToolInfo {
  name: string;
  title?: string;
  description?: string;
  inputSchema: JSONSchema7;
  outputSchema?: JSONSchema7;
  annotations?: Record<string, unknown>;
  serverName: string;
}

export interface MCPPromptInfo {
  name: string;
  description?: string;
  arguments?: {
    name: string;
    description?: string;
    required?: boolean;
  }[];
  serverName: string;
}

export interface MCPToolResult {
  content: { type: string; text?: string; [key: string]: unknown }[];
  structuredContent?: unknown;
  isError?: boolean;
}

export class MCPClientManager {
  private clients = new Map<string, Client>();
  private tools = new Map<string, MCPToolInfo>();
  private prompts = new Map<string, MCPPromptInfo>();
  private serverInstructions = new Map<string, string>();
  private toolTimeout: number;

  constructor(toolTimeout = 10000) {
    this.toolTimeout = toolTimeout;
  }

  /**
   * Initialize MCP servers according to configuration
   */
  async initializeServers(servers: Record<string, MCPServerConfig>): Promise<void> {
    const initPromises = Object.entries(servers)
      .filter(([_, config]) => config.enabled !== false)
      .map(([name, config]) => this.initializeServer(name, config));

    await Promise.all(initPromises);
  }

  /**
   * Initialize a single MCP server
   */
  private async initializeServer(serverName: string, config: MCPServerConfig): Promise<void> {
    try {
      // Create transport based on server type
      const transport = await this.createTransport(config);
      
      // Create MCP client with proper capabilities
      const client = new Client(
        { 
          name: "ai-agent", 
          version: "1.0.0",
          title: "AI Agent Production Client"
        },
        { 
          capabilities: { 
            tools: {},
            prompts: {} // Enable prompts capability
          } 
        }
      );

      // Connect to server
      await client.connect(transport as Parameters<typeof client.connect>[0]);
      this.clients.set(serverName, client);

      // Get server instructions if available
      const instructions = 'getInstructions' in client && typeof client.getInstructions === 'function' ? client.getInstructions() : undefined;
      if (typeof instructions === 'string' && instructions.trim() !== '') {
        this.serverInstructions.set(serverName, instructions);
        console.error(`[mcp] Retrieved instructions from server '${serverName}'`);
      }

      // Discover tools
      const toolsResult = await client.listTools();
      this.processToolsRecursively(toolsResult.tools, 0, serverName);

      // Discover prompts (optional capability)
      try {
        const promptsResult = await client.listPrompts();
        this.processPromptsRecursively(promptsResult.prompts, 0, serverName);
      } catch (error) {
        // Prompts are optional - server may not support them
        console.error(`[mcp] Server '${serverName}' does not support prompts: ${String(error)}`);
      }

      console.error(`[mcp] Initialized server '${serverName}' with ${String(toolsResult.tools.length)} tools`);

    } catch (error) {
      throw new Error(`Failed to initialize MCP server '${serverName}': ${String(error)}`);
    }
  }

  /**
   * Create transport based on configuration with proper environment resolution
   */
  private async createTransport(config: MCPServerConfig): Promise<unknown> {
    const resolvedHeaders = this.resolveEnvironmentVariables(config.headers ?? {});
    const resolvedEnv = this.resolveEnvironmentVariables(config.env ?? {});

    switch (config.type) {
      case 'stdio':
        if (typeof config.command !== 'string' || config.command.trim() === '') {
          throw new Error('Command is required for stdio transport');
        }
        return new StdioClientTransport({
          command: config.command,
          args: config.args ?? [],
          env: resolvedEnv,
          stderr: 'pipe' // Capture stderr for logging
        });

      case 'websocket':
        if (typeof config.url !== 'string' || config.url.trim() === '') {
          throw new Error('URL is required for websocket transport');
        }
        return await createWebSocketTransport(config.url, resolvedHeaders);

      case 'http':
        if (typeof config.url !== 'string' || config.url.trim() === '') {
          throw new Error('URL is required for http transport');
        }
        return new StreamableHTTPClientTransport(
          new URL(config.url),
          { requestInit: { headers: resolvedHeaders } }
        );

      case 'sse':
        if (typeof config.url !== 'string' || config.url.trim() === '') {
          throw new Error('URL is required for sse transport');
        }
        return new SSEClientTransport(
          new URL(config.url),
          resolvedHeaders
        );

      default:
        throw new Error(`Unsupported transport type: ${String(config.type)}`);
    }
  }

  /**
   * Resolve ${VAR} placeholders in configuration values
   * Per spec: expand for all strings except under mcpServers.*.env|headers where preserved for server process
   */
  private resolveEnvironmentVariables(obj: Record<string, string>): Record<string, string> {
    const resolved: Record<string, string> = {};
    
    this.processEnvironmentVariablesRecursively(Object.entries(obj), 0, resolved);
    
    return resolved;
  }

  /**
   * Process tools recursively to avoid for-of loops
   */
  private processToolsRecursively(tools: unknown[], index: number, serverName: string): void {
    if (index >= tools.length) {
      return;
    }
    
    const tool = tools[index] as {
      name: string;
      title?: string;
      description?: string;
      inputSchema?: JSONSchema7;
      outputSchema?: JSONSchema7;
      annotations?: Record<string, unknown>;
    };
    
    // Handle legacy servers that return 'parameters' instead of 'inputSchema'
    const inputSchema = tool.inputSchema ?? (tool as Record<string, unknown>)['parameters'] as JSONSchema7 | undefined;
    
    const toolInfo: MCPToolInfo = {
      name: tool.name,
      title: tool.title,
      description: tool.description,
      inputSchema: inputSchema ?? { type: 'object', properties: {} },
      outputSchema: tool.outputSchema,
      annotations: tool.annotations,
      serverName,
    };
    
    this.tools.set(tool.name, toolInfo);
    this.processToolsRecursively(tools, index + 1, serverName);
  }

  /**
   * Process prompts recursively to avoid for-of loops
   */
  private processPromptsRecursively(prompts: unknown[], index: number, serverName: string): void {
    if (index >= prompts.length) {
      return;
    }
    
    const prompt = prompts[index] as {
      name: string;
      description?: string;
      arguments?: {
        name: string;
        description?: string;
        required?: boolean;
      }[];
    };
    
    const promptInfo: MCPPromptInfo = {
      name: prompt.name,
      description: prompt.description,
      arguments: prompt.arguments,
      serverName,
    };
    
    this.prompts.set(prompt.name, promptInfo);
    this.processPromptsRecursively(prompts, index + 1, serverName);
  }

  /**
   * Process environment variables recursively to avoid for-of loops
   */
  private processEnvironmentVariablesRecursively(
    entries: [string, string][],
    index: number,
    resolved: Record<string, string>
  ): void {
    if (index >= entries.length) {
      return;
    }
    
    const [key, value] = entries[index] ?? ['', ''];
    const resolvedValue = value.replace(/\$\{([^}]+)\}/g, (_, varName) => {
      return process.env[varName as string] ?? '';
    });
    
    // Omit empty values per spec
    if (resolvedValue.trim() !== '') {
      resolved[key] = resolvedValue;
    }
    
    this.processEnvironmentVariablesRecursively(entries, index + 1, resolved);
  }

  /**
   * Process server instructions recursively to avoid for-of loops
   */
  private processServerInstructionsRecursively(
    entries: [string, string][],
    index: number,
    instructions: string[]
  ): void {
    if (index >= entries.length) {
      return;
    }
    
    const [serverName, serverInstruction] = entries[index] ?? ['', ''];
    instructions.push(`## TOOL ${serverName.toUpperCase()} INSTRUCTIONS\n\n${serverInstruction}`);
    this.processServerInstructionsRecursively(entries, index + 1, instructions);
  }

  /**
   * Process prompt instructions recursively to avoid for-of loops
   */
  private processPromptInstructionsRecursively(
    entries: [string, MCPPromptInfo][],
    index: number,
    instructions: string[]
  ): void {
    if (index >= entries.length) {
      return;
    }
    
    const [, promptInfo] = entries[index] ?? ['', { name: '', serverName: '' }];
    if (promptInfo.description !== undefined && promptInfo.description.trim() !== '') {
      instructions.push(`## TOOL ${promptInfo.name.toUpperCase()} INSTRUCTIONS\n\n${promptInfo.description}`);
    }
    this.processPromptInstructionsRecursively(entries, index + 1, instructions);
  }

  /**
   * Execute MCP tool with timeout and proper error handling
   */
  async callTool(toolName: string, args: Record<string, unknown>): Promise<MCPToolResult> {
    const toolInfo = this.tools.get(toolName);
    if (toolInfo === undefined) {
      throw new Error(`Tool '${toolName}' not found`);
    }

    const client = this.clients.get(toolInfo.serverName);
    if (client === undefined) {
      throw new Error(`Client for server '${toolInfo.serverName}' not found`);
    }

    // Implement per-tool timeout as specified
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => { reject(new Error('Tool execution timed out')); }, this.toolTimeout);
    });

    try {
      const result = await Promise.race([
        client.callTool({ name: toolName, arguments: args }),
        timeoutPromise
      ]) as {
        content?: { type: string; text?: string; [key: string]: unknown }[];
        structuredContent?: unknown;
        isError?: boolean;
      };

      // Return structured result matching spec
      return {
        content: (result.content ?? []) as { type: string; text?: string; [key: string]: unknown }[],
        structuredContent: result.structuredContent,
        isError: result.isError === true
      };

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error(`[mcp] Tool '${toolName}' failed: ${errorMessage}`);
      
      // Return error as failed tool result per spec - don't throw
      return {
        content: [{ type: 'text', text: `Tool execution failed: ${errorMessage}` }],
        isError: true
      };
    }
  }

  /**
   * Get all discovered tools
   */
  getTools(): Map<string, MCPToolInfo> {
    return new Map(this.tools);
  }

  /**
   * Get all discovered prompts  
   */
  getPrompts(): Map<string, MCPPromptInfo> {
    return new Map(this.prompts);
  }

  /**
   * Build combined instructions string for system prompt according to spec format
   */
  buildInstructions(): string {
    const instructions: string[] = [];

    // Add server instructions with proper headers
    this.processServerInstructionsRecursively(
      Array.from(this.serverInstructions.entries()), 
      0, 
      instructions
    );

    // Add prompt descriptions as additional instructions  
    this.processPromptInstructionsRecursively(
      Array.from(this.prompts.entries()),
      0,
      instructions
    );

    return instructions.join('\n\n');
  }

  /**
   * Clean shutdown of all MCP clients
   */
  async cleanup(): Promise<void> {
    const closePromises = Array.from(this.clients.entries()).map(async ([name, client]) => {
      try {
        await client.close();
        console.error(`[mcp] Closed client: ${name}`);
      } catch (error) {
        console.error(`[mcp] Error closing client ${name}: ${String(error)}`);
      }
    });

    await Promise.all(closePromises);
    this.clients.clear();
    this.tools.clear();
    this.prompts.clear();
    this.serverInstructions.clear();
  }
}