import { fileURLToPath } from 'node:url';

import type { Configuration, MCPServerConfig } from '../types.js';

const resolveFsServerScript = (): string => {
  const scriptUrl = new URL('../../mcp/fs/fs-mcp-server.js', import.meta.url);
  return fileURLToPath(scriptUrl);
};

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
