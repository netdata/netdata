import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

import type { Configuration, MCPServerConfig } from '../types.js';

const resolveFsServerScript = (): string => {
  const scriptUrl = new URL('../../mcp/fs/fs-mcp-server.js', import.meta.url);
  return fileURLToPath(scriptUrl);
};

export function assertToolOutputFsServerAvailable(): void {
  const scriptPath = resolveFsServerScript();
  try {
    fs.accessSync(scriptPath, fs.constants.R_OK);
    const stat = fs.statSync(scriptPath);
    if (!stat.isFile()) {
      throw new Error('not a file');
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`tool_output filesystem MCP server missing at ${scriptPath}: ${message}`);
  }
}

export function buildToolOutputFsServerConfig(
  baseConfig: Configuration,
  rootDir: string,
): { name: string; config: Configuration } {
  const serverName = 'tool_output_fs';
  const serverScript = resolveFsServerScript();
  const serverConfig: MCPServerConfig = {
    type: 'stdio',
    command: process.execPath,
    args: [serverScript, '--no-rgrep-root', '--no-tree-root', rootDir],
    toolsAllowed: ['Read', 'Grep'],
  };
  const config: Configuration = {
    ...baseConfig,
    mcpServers: { [serverName]: serverConfig },
    restTools: undefined,
    openapiSpecs: undefined,
  };
  return { name: serverName, config };
}
