import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, ErrorCode, type CallToolResult, McpError } from '@modelcontextprotocol/sdk/types.js';

const server = new McpServer({
  name: 'test-mcp-server',
  version: '0.2.0',
});

const noopResult: CallToolResult = {
  content: [
    {
      type: 'text',
      text: 'noop',
    },
  ],
};

server.registerTool(
  'test',
  {
    description: 'Echo helper used for deterministic tests.',
    inputSchema: {},
  },
  async () => noopResult,
);

server.registerTool(
  'test-summary',
  {
    description: 'Summarises values for deterministic tests.',
    inputSchema: {},
  },
  async () => noopResult,
);

const longPayload = '#'.repeat(1024);

server.server.setRequestHandler(CallToolRequestSchema, async (request, extra) => {
  const { name, arguments: rawArgs } = request.params;
  const args = (rawArgs ?? {}) as Record<string, unknown>;

  if (name === 'test') {
    const textUnknown = args.text;
    const payload = typeof textUnknown === 'string' ? textUnknown : JSON.stringify(textUnknown);

    if (payload === 'trigger-mcp-failure') {
      throw new McpError(ErrorCode.InternalError, 'Simulated MCP tool failure');
    }

    if (payload === 'trigger-timeout') {
      await new Promise((resolve) => { setTimeout(resolve, 1500); });
    }

    if (payload === 'long-output') {
      return {
        content: [
          { type: 'text', text: longPayload },
        ],
      };
    }

    return {
      content: [
        { type: 'text', text: payload },
      ],
    };
  }

  if (name === 'test-summary') {
    const valueUnknown = args.value;
    const normalized = typeof valueUnknown === 'string' ? valueUnknown : JSON.stringify(valueUnknown);
    return {
      content: [
        {
          type: 'text',
          text: `# Summary\n\nReceived: ${normalized}`,
        },
      ],
    };
  }

  throw new McpError(ErrorCode.InvalidParams, `Unknown tool ${name}`);
});

async function main(): Promise<void> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  // eslint-disable-next-line no-console
  console.error(`test-mcp-server failed: ${message}`);
  process.exitCode = 1;
});
