export class MCPBootstrapError extends Error {
}
export async function bootstrapMCP(cfg, toolServerNames) {
    const out = {};
    for (const serverName of toolServerNames) {
        const server = cfg.mcpServers[serverName];
        if (!server)
            throw new MCPBootstrapError(`MCP server not found: ${serverName}`);
        await ensureReachable(serverName, server);
        const toolInfo = await discoverTools(serverName, server);
        out[serverName] = toolInfo;
    }
    return out;
}
async function ensureReachable(serverName, server) {
    // Placeholder reachability checks. Real implementation will open stdio/http/ws/sse connection.
    if (!server.type)
        throw new MCPBootstrapError(`Invalid MCP server type for ${serverName}`);
}
async function discoverTools(serverName, server) {
    // Placeholder: in a real implementation, query the MCP server for tools and any instructions.
    // Here we return empty schemas and no instructions so the rest of the pipeline can proceed.
    return {
        serverName,
        schemas: [],
        instructions: undefined,
    };
}
