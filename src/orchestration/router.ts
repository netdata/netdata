import type { ToolExecuteOptions, ToolExecuteResult, ToolKind } from '../tools/types.js';
import type { MCPTool } from '../types.js';
import type { RouterToolConfig } from './types.js';

import { renderPromptTemplate } from '../prompts/templates.js';
import { ToolExecutionError } from '../tools/tool-errors.js';
import { ToolProvider } from '../tools/types.js';
import { parseJsonRecordStrict } from '../utils.js';

export interface RouterToolProviderOptions {
  config: RouterToolConfig;
}

export class RouterToolProvider extends ToolProvider {
  static readonly TOOL_NAMESPACE = 'router';
  static readonly TOOL_NAME = 'handoff-to';
  static readonly FULL_TOOL_NAME = `${RouterToolProvider.TOOL_NAMESPACE}__${RouterToolProvider.TOOL_NAME}`;

  readonly kind: ToolKind = 'agent';
  readonly namespace = 'agent';
  private readonly config: RouterToolConfig;
  private readonly toolSchema?: { description?: string; inputSchema: Record<string, unknown> };

  constructor(opts: RouterToolProviderOptions) {
    super();
    this.config = opts.config;
    this.toolSchema = this.config.destinations.length > 0 ? this.renderToolSchema() : undefined;
  }

  static getToolName(): string {
    return RouterToolProvider.FULL_TOOL_NAME;
  }

  listTools(): MCPTool[] {
    if (this.config.destinations.length === 0) {
      return [];
    }
    const schema = this.toolSchema ?? this.renderToolSchema();
    return [
      {
        name: RouterToolProvider.FULL_TOOL_NAME,
        description: schema.description ?? 'Route the request to a destination agent, optionally with a note.',
        inputSchema: schema.inputSchema,
      },
    ];
  }

  hasTool(name: string): boolean {
    return name === RouterToolProvider.FULL_TOOL_NAME;
  }

  override resolveToolIdentity(name: string): {
    namespace: string;
    tool: string;
  } {
    if (name === RouterToolProvider.FULL_TOOL_NAME) {
      return {
        namespace: RouterToolProvider.TOOL_NAMESPACE,
        tool: RouterToolProvider.TOOL_NAME,
      };
    }
    return { namespace: RouterToolProvider.TOOL_NAMESPACE, tool: name };
  }

  override resolveLogProvider(): string {
    return RouterToolProvider.TOOL_NAMESPACE;
  }

  async execute(
    name: string,
    parameters: Record<string, unknown>,
    _opts?: ToolExecuteOptions,
  ): Promise<ToolExecuteResult> {
    if (name !== RouterToolProvider.FULL_TOOL_NAME) {
      throw new ToolExecutionError('unknown_tool', `Unknown tool: ${name}`, {
        details: { toolName: name },
      });
    }
    const agent = parameters.agent;
    if (typeof agent !== "string") {
      throw new ToolExecutionError('invalid_parameters', 'agent must be a string', {
        details: { toolName: name, field: 'agent' },
      });
    }
    if (!this.config.destinations.includes(agent)) {
      throw new ToolExecutionError('invalid_parameters', `Unknown agent: ${agent}`, {
        details: { toolName: name, field: 'agent', agent },
      });
    }
    const message = parameters.message;
    if (message !== undefined && typeof message !== 'string') {
      throw new ToolExecutionError('invalid_parameters', 'message must be a string when provided', {
        details: { toolName: name, field: 'message' },
      });
    }
    await Promise.resolve();
    return {
      ok: true,
      result: `Routed to agent: ${agent}`,
      latencyMs: 0,
      kind: this.kind,
      namespace: this.namespace,
    };
  }

  override getInstructions(): string {
    if (this.config.destinations.length === 0) {
      return '';
    }
    return renderPromptTemplate('routerInstructions', { destinations: this.config.destinations });
  }

  private renderToolSchema(): { description?: string; inputSchema: Record<string, unknown> } {
    const raw = renderPromptTemplate('toolSchemaRouter', {
      destinations: this.config.destinations,
    });
    const parsed = parseJsonRecordStrict(raw, 'router-handoff tool schema template');
    const description = typeof parsed.description === 'string' ? parsed.description : undefined;
    return { description, inputSchema: parsed };
  }
}
