#!/usr/bin/env node

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

const varName = process.argv[2] || 'BRAVE_API_KEY';
const childEnv = { [varName]: process.env[varName] || '' };

const client = new Client({ name: 'env-test-client', version: '1.0.0' }, { capabilities: { tools: {} } });
const transport = new StdioClientTransport({
  command: 'node',
  args: ['codex/tools/env-test-server.mjs'],
  env: childEnv,
  stderr: 'pipe',
});

transport.stderr?.on('data', (d) => process.stderr.write(`[server-stderr] ${d}\n`));

await client.connect(transport);
const tools = await client.listTools();
const hasTool = tools.tools.some((t) => t.name === 'require-env');
if (!hasTool) {
  console.error('Tool require-env not found');
  process.exit(1);
}
try {
  const resp = await client.callTool({ name: 'require-env', arguments: { name: varName } });
  const text = (resp.content || []).map((c) => (c.type === 'text' ? c.text : `[${c.type}]`)).join('');
  process.stdout.write(String(text || '(no content)'));
  process.exit(0);
} catch (e) {
  console.error(`callTool failed: ${e instanceof Error ? e.message : String(e)}`);
  process.exit(2);
}
