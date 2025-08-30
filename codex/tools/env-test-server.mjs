#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { z } from 'zod';

const server = new McpServer({ name: 'env-test-server', version: '1.0.0' });

server.registerTool(
  'require-env',
  {
    description: 'Returns the value of a required environment variable',
    inputSchema: {
      name: z.string().default('BRAVE_API_KEY').describe('Environment variable name to require'),
    },
  },
  async ({ name }) => {
    const val = process.env[name];
    if (!val) {
      throw new Error(`Missing env var: ${name}`);
    }
    const preview = val.length > 8 ? `${val.slice(0, 4)}…${val.slice(-4)}` : val;
    return { content: [{ type: 'text', text: `OK ${name}=${preview}` }] };
  }
);

const transport = new StdioServerTransport();
await server.connect(transport);
