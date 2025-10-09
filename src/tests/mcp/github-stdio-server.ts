import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, type CallToolResult, McpError, ErrorCode } from '@modelcontextprotocol/sdk/types.js';

const server = new McpServer({
  name: 'github-mcp-server',
  version: '0.2.0',
});

server.registerTool(
  'search_code',
  {
    description: 'Deterministic GitHub code search stub for coverage.',
    inputSchema: {},
  },
  async () => {
    const result: CallToolResult = {
      content: [
        { type: 'text', text: 'noop' },
      ],
    };
    return result;
  },
);

server.server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: rawArgs } = request.params;
  if (name !== 'search_code') {
    throw new McpError(ErrorCode.InvalidParams, `Unknown tool ${name}`);
  }
  const args = (rawArgs ?? {}) as Record<string, unknown>;
  const payload = JSON.stringify({
    q: args.q,
    query: args.query,
    repo: args.repo,
    path: args.path,
    language: args.language,
  });
  return {
    content: [
      { type: 'text', text: payload },
    ],
  } satisfies CallToolResult;
});

async function main(): Promise<void> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`github-mcp-server failed: ${message}`);
  process.exitCode = 1;
});
