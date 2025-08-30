#!/usr/bin/env node

/**
 * Test MCP Server with Instructions
 * This server demonstrates how MCP instructions should work
 */

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { z } from 'zod';

const SERVER_NAME = 'test-server';

// Create server with instructions
const server = new McpServer({
  name: SERVER_NAME,
  version: '1.0.0',
}, {
  // THIS IS THE KEY: MCP server instructions that should be added to system prompt
  instructions: `This test server provides sample tools for demonstration purposes.

## Available Tools:
- **get-info**: Returns server information and capabilities
- **echo-message**: Echoes back the provided message with formatting

## Usage Guidelines:
- Always check server capabilities before using tools
- Use descriptive messages when echoing for better results
- The server responds in a structured format for easy parsing

## Important Notes:
- This is a test server for validating MCP instruction handling
- Instructions like these should be automatically integrated into the LLM's system prompt
- If you can see these instructions, the MCP client is working correctly!`
});

// Add test tools
server.tool(
  'get-info',
  'Get information about this test server and its capabilities',
  {},
  async () => {
    return {
      content: [
        {
          type: 'text',
          text: JSON.stringify({
            serverName: SERVER_NAME,
            version: '1.0.0',
            capabilities: ['tool-calling', 'instructions'],
            message: 'This response confirms the MCP tool execution is working correctly!',
            timestamp: new Date().toISOString()
          }, null, 2)
        }
      ]
    };
  }
);

server.tool(
  'echo-message',
  'Echo back a message with formatting and timestamp',
  {
    message: z.string().describe('The message to echo back')
  },
  async ({ message }) => {
    return {
      content: [
        {
          type: 'text',
          text: JSON.stringify({
            originalMessage: message,
            echoedAt: new Date().toISOString(),
            serverName: SERVER_NAME,
            formattedMessage: `📢 ECHO: ${message}`,
            instructionTest: 'If you can see this, MCP instruction integration is working!'
          }, null, 2)
        }
      ]
    };
  }
);

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error('Test MCP Server with instructions running on stdio');
}

main().catch(error => {
  console.error('Fatal error in test server:', error);
  process.exit(1);
});