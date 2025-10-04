import type { ZodRawShape } from 'zod';
import { z } from 'zod';

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, ErrorCode, McpError } from '@modelcontextprotocol/sdk/types.js';

const server = new McpServer({
  name: 'toy-mcp-server',
  version: '0.1.0',
});

const echoShape = { text: z.string().describe('Text to echo back.') } as unknown as ZodRawShape;
const summaryShape = { value: z.string().describe('Value to acknowledge in the summary.') } as unknown as ZodRawShape;

server.tool(
  'toy',
  echoShape,
  async (args) => {
    const text = typeof args.text === 'string' ? args.text : JSON.stringify(args.text);
    const payload = text;
    if (payload === 'trigger-mcp-failure') {
      throw new Error('Simulated MCP tool failure');
    }
    return {
      content: [
        { type: 'text', text: payload },
      ],
    };
  }
);

server.tool(
  'toy-summary',
  summaryShape,
  async (args) => {
    const value = typeof args.value === 'string' ? args.value : JSON.stringify(args.value);
    const normalized = value;
    return {
      content: [
        {
          type: 'text',
          text: `# Summary\n\nReceived: ${normalized}`,
        },
      ],
    };
  }
);

// Override callTool handler to allow lightweight argument validation without Zod strictness.
server.server.setRequestHandler(CallToolRequestSchema, async (request, extra) => {
  const toolName = request.params.name;
  const args = request.params.arguments ?? {};
  if (toolName === 'toy') {
    const rawText = (args as { text?: unknown }).text;
    const payload = typeof rawText === 'string' ? rawText : JSON.stringify(rawText);
    if (payload === 'trigger-mcp-failure') {
      throw new McpError(ErrorCode.InternalError, 'Simulated MCP tool failure');
    }
    return {
      content: [
        { type: 'text', text: payload },
      ],
    };
  }
  if (toolName === 'toy-summary') {
    const rawValue = (args as { value?: unknown }).value;
    const normalized = typeof rawValue === 'string' ? rawValue : JSON.stringify(rawValue);
    return {
      content: [
        {
          type: 'text',
          text: `# Summary\n\nReceived: ${normalized}`,
        },
      ],
    };
  }
  throw new McpError(ErrorCode.InvalidParams, `Unknown tool ${toolName}`);
});

async function main(): Promise<void> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  // eslint-disable-next-line no-console
  console.error(`toy-mcp-server failed: ${message}`);
  process.exitCode = 1;
});
