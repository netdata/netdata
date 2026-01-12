import type { ToolExecuteOptions, ToolExecuteResult, ToolKind } from '../tools/types.js';
import type { MCPTool } from '../types.js';
import type { RouterToolConfig } from './types.js';

import { ToolProvider } from '../tools/types.js';

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

  constructor(opts: RouterToolProviderOptions) {
    super();
    this.config = opts.config;
  }

  static getToolName(): string {
    return RouterToolProvider.FULL_TOOL_NAME;
  }

  listTools(): MCPTool[] {
    if (this.config.destinations.length === 0) {
      return [];
    }
    return [
      {
        name: RouterToolProvider.FULL_TOOL_NAME,
        description:
          'Route the request to a destination agent, optionally with a note.',
        inputSchema: {
          type: 'object',
          properties: {
            agent: {
              type: 'string',
              enum: this.config.destinations,
              description: 'The destination agent to route to',
            },
            message: {
              type: 'string',
              description:
                'Optional context for the destination agent (plain text)',
            },
          },
          required: ['agent'],
          additionalProperties: false,
        },
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
      return {
        ok: false,
        error: `Unknown tool: ${name}`,
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    const agent = parameters.agent;
    if (typeof agent !== "string") {
      return {
        ok: false,
        error: 'agent must be a string',
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    if (!this.config.destinations.includes(agent)) {
      return {
        ok: false,
        error: `Unknown agent: ${agent}`,
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    const message = parameters.message;
    if (message !== undefined && typeof message !== 'string') {
      return {
        ok: false,
        error: 'message must be a string when provided',
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
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
    return `Use ${RouterToolProvider.FULL_TOOL_NAME} to route to one of: ${this.config.destinations.join(', ')}`;
  }
}
