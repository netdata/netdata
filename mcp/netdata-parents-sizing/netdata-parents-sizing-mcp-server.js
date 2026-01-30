#!/usr/bin/env node
/**
 * Netdata Parents Sizing MCP stdio server (no external deps).
 *
 * Usage: node netdata-parents-sizing-mcp-server.js
 *
 * Tools:
 * - calculate_parent_sizing(inputs): Calculate CPU and RAM requirements for Netdata Parent nodes
 *   based on infrastructure metrics and configuration.
 *
 * Based on the Netdata Parents Sizing Guide spreadsheet.
 */

// -------------------------- Utility: JSON-RPC over stdio --------------------------
const STDIN = process.stdin;
const STDOUT = process.stdout;

function send(message) {
  const json = JSON.stringify(message);
  STDOUT.write(json + '\n');
}

function respond(id, result) { send({ jsonrpc: '2.0', id, result }); }
function respondError(id, code, message, data) {
  const error = data === undefined ? { code, message } : { code, message, data };
  send({ jsonrpc: '2.0', id, error });
}

// -------------------------- Server Instructions --------------------------
const SERVER_INSTRUCTIONS = `Netdata Parents Sizing Calculator - Calculate CPU and RAM requirements for Netdata Parent nodes.

INPUTS:
- max_concurrently_connected_nodes (required): Number of concurrently monitored nodes at peak time
- metrics_per_node: Metrics per second per node (default: 5000)
- ephemerality: Infrastructure stability factor (default: 2)
- clustered_parent: Whether parents are clustered (default: true)
- ml_enabled: Machine Learning enabled (default: true)
- mem_safety_margin: Memory headroom percentage (default: 0.30 = 30%)
- cpu_safety_margin: CPU headroom for queries percentage (default: 0.40 = 40%)

GUIDELINES:
- Simple nodes (bare metal, VMs): ~3000 metrics/s
- Fat Kubernetes nodes: ~10000 metrics/s
- Stable infrastructure: ephemerality = 1
- Daily autoscaling k8s: ephemerality = 10
- Clustered parents stream to each other, requiring more resources

OUTPUT:
Returns breakdown of metrics, raw resource needs, and final recommendations with safety margins applied.`;

// -------------------------- Sizing Logic (from Netdata Parents Sizing Guide) --------------------------

/**
 * Calculates Netdata Parent sizing based on infrastructure metrics.
 * Logic derived from 'Netdata Parents Sizing Guide', specifically the 'Your Own' column.
 */
function calculateSizing(inputs) {
    // Apply defaults
    const {
        max_concurrently_connected_nodes,
        metrics_per_node = 5000,
        ephemerality = 2,
        clustered_parent = true,
        ml_enabled = true,
        mem_safety_margin = 0.30,
        cpu_safety_margin = 0.40
    } = inputs;

    // Validate required parameter
    if (typeof max_concurrently_connected_nodes !== 'number' || max_concurrently_connected_nodes <= 0) {
        throw new Error('max_concurrently_connected_nodes must be a positive number');
    }

    // Validate optional parameters
    if (typeof metrics_per_node !== 'number' || metrics_per_node <= 0) {
        throw new Error('metrics_per_node must be a positive number');
    }
    if (typeof ephemerality !== 'number' || ephemerality < 1) {
        throw new Error('ephemerality must be a number >= 1');
    }
    if (typeof clustered_parent !== 'boolean') {
        throw new Error('clustered_parent must be a boolean');
    }
    if (typeof ml_enabled !== 'boolean') {
        throw new Error('ml_enabled must be a boolean');
    }

    // Convert booleans to numeric for calculations
    const is_clustered = clustered_parent ? 1 : 0;
    const ml_factor = ml_enabled ? 1 : 0;

    // Constants (Rows C25:C30 and C34:C36 from spreadsheet)
    const CONSTANTS = {
        // Memory (KiB per unit)
        MEM_METRIC_RETENTION: 1,
        MEM_METRIC_COLLECTED: 20,
        MEM_METRIC_ML: 5,
        MEM_NODE_RETENTION: 12,
        MEM_NODE_RECEIVED: 512,
        MEM_NODE_SENT: 10240,

        // CPU (Cores per 1 Million metrics/s)
        CPU_INGESTION: 2,
        CPU_ML: 2,
        CPU_STREAMING: 2
    };

    // --- Intermediate Calculations (Rows 15-20) ---

    // Metrics
    const metrics_curr_collected = max_concurrently_connected_nodes * metrics_per_node;
    const metrics_retention = metrics_curr_collected * ephemerality;
    const metrics_ml = metrics_curr_collected * ml_factor;

    // Nodes
    const nodes_retention = max_concurrently_connected_nodes * ephemerality;
    const nodes_received = max_concurrently_connected_nodes;
    const nodes_sent = max_concurrently_connected_nodes * is_clustered;

    // --- Resource Calculations (Rows 25-36) ---

    // Memory (MiB) - Formula: (Count * Cost_KiB) / 1024
    const mem_usage_mib = (
        (metrics_retention * CONSTANTS.MEM_METRIC_RETENTION) +
        (metrics_curr_collected * CONSTANTS.MEM_METRIC_COLLECTED) +
        (metrics_ml * CONSTANTS.MEM_METRIC_ML) +
        (nodes_retention * CONSTANTS.MEM_NODE_RETENTION) +
        (nodes_received * CONSTANTS.MEM_NODE_RECEIVED) +
        (nodes_sent * CONSTANTS.MEM_NODE_SENT)
    ) / 1024;

    // CPU (Cores) - Formula: (Metrics / 1,000,000) * Cost_Factor
    const millions_metrics = metrics_curr_collected / 1_000_000;

    // Streaming cost applies to ingestion volume, but only if clustered
    const cpu_usage_cores = (
        (millions_metrics * CONSTANTS.CPU_INGESTION) +
        ((metrics_ml / 1_000_000) * CONSTANTS.CPU_ML) +
        (millions_metrics * CONSTANTS.CPU_STREAMING * is_clustered)
    );

    // --- Final Outputs (Rows 39-40) ---
    // Total = Subtotal / (1 - Margin)

    const final_cpu_cores = cpu_usage_cores / (1 - cpu_safety_margin);
    const final_mem_mib = mem_usage_mib / (1 - mem_safety_margin);
    const final_mem_gib = final_mem_mib / 1024;

    // Round CPU to nearest even number (cores come in 2, 4, 6, 8...)
    const roundCpu = (n) => Math.ceil(n / 2) * 2;
    // Round RAM to multiples of 4 GiB, minimum 4 GiB
    const roundRam = (n) => Math.max(4, Math.ceil(n / 4) * 4);

    return {
        inputs: {
            max_concurrently_connected_nodes,
            metrics_per_node,
            ephemerality,
            clustered_parent,
            ml_enabled,
            mem_safety_margin,
            cpu_safety_margin
        },
        breakdown: {
            total_metrics_per_second: metrics_curr_collected,
            metrics_retention: metrics_retention,
            metrics_ml: metrics_ml,
            nodes_retention: nodes_retention,
            nodes_received: nodes_received,
            nodes_sent: nodes_sent,
            raw_memory_needed_mib: parseFloat(mem_usage_mib.toFixed(2)),
            raw_cpu_needed_cores: parseFloat(cpu_usage_cores.toFixed(4))
        },
        recommendation: {
            cpu_cores: roundCpu(final_cpu_cores),
            ram_gib: roundRam(final_mem_gib),
            cpu_cores_exact: parseFloat(final_cpu_cores.toFixed(2)),
            ram_gib_exact: parseFloat(final_mem_gib.toFixed(2)),
            details: `Recommended: ${roundCpu(final_cpu_cores)} CPU Cores and ${roundRam(final_mem_gib)} GiB RAM`
        }
    };
}

// -------------------------- Tool Implementation --------------------------
function formatNumber(n) {
    if (n >= 1_000_000) {
        const millions = n / 1_000_000;
        return millions % 1 === 0 ? `${millions}M` : `${millions.toFixed(1)}M`;
    }
    if (n >= 1_000) {
        const thousands = n / 1_000;
        return thousands % 1 === 0 ? `${thousands}k` : `${thousands.toFixed(1)}k`;
    }
    return String(n);
}

function formatResponse(result) {
    const {
        max_concurrently_connected_nodes,
        metrics_per_node,
        ephemerality,
        clustered_parent,
        ml_enabled,
        mem_safety_margin,
        cpu_safety_margin
    } = result.inputs;

    const {
        total_metrics_per_second,
        metrics_retention,
        nodes_retention,
        raw_memory_needed_mib,
        raw_cpu_needed_cores
    } = result.breakdown;

    const cpu_cores = result.recommendation.cpu_cores;
    const ram_gib = result.recommendation.ram_gib;
    const mem_margin_pct = Math.round(mem_safety_margin * 100);
    const cpu_margin_pct = Math.round(cpu_safety_margin * 100);

    // Calculate actual free percentages after rounding
    const raw_ram_gib = raw_memory_needed_mib / 1024;
    const actual_cpu_free_pct = Math.round((1 - raw_cpu_needed_cores / cpu_cores) * 100);
    const actual_ram_free_pct = Math.round((1 - raw_ram_gib / ram_gib) * 100);

    return `# Netdata Parent Sizing: ${max_concurrently_connected_nodes} Nodes

## Configuration Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| **Concurrent Nodes** | ${max_concurrently_connected_nodes} | Distinct monitored hosts (physical servers, VMs, SNMP devices, vnodes). Containers are not counted as nodes. |
| **Metrics per Node** | ${metrics_per_node} | Average metrics/second per node across all collection types. Reference values: bare-metal/VMs ~5k, Kubernetes nodes ~20k, SNMP devices ~300. |
| **Ephemerality** | ${ephemerality}x | Ratio of historical to active nodes in retention. 1 = static infrastructure, 2 = full rotation within retention period, higher = dynamic environments. |
| **Parent Clustering** | ${clustered_parent} | Enable when this parent streams to other parents. Disable for standalone deployments. |
| **Machine Learning** | ${ml_enabled} | Enable for parent-side anomaly detection. Disable if child nodes perform ML locally (anomaly data propagates via streaming). |
| **Memory Margin** | ${mem_margin_pct}% | Reserved for OS and other processes. Values below 10% risk OOM conditions. |
| **CPU Margin** | ${cpu_margin_pct}% | Reserved for queries and other processes. Values below 20% may cause query latency. |

## Estimated Workload

| Metric | Value |
|--------|-------|
| **Peak Ingestion Rate** | ${formatNumber(total_metrics_per_second)} metrics/s |
| **Max Metrics in TSDB** | ${formatNumber(metrics_retention)} metrics |
| **Max Nodes in TSDB** | ${nodes_retention} nodes |

## Recommended Sizing (Per Parent)

The following sizing recommendations are NOT resource consumption. They are recommended VM allocation per Parent (2x are needed for HA).

| Resource | Specification |
|----------|---------------|
| **CPU** | ${cpu_cores} cores |
| **RAM** | ${ram_gib} GiB |

*Included free resources: ${actual_cpu_free_pct}% CPU, ${actual_ram_free_pct}% RAM.*

---

Rounding:
- CPU cores are rounded up to the nearest even number (2, 4, 6, 8...).
- RAM is rounded up to multiples of 4 GiB, minimum 4 GiB.

Present these parameters to users and recalculate if adjustments are needed.
`;
}

async function toolCalculateParentSizing(args) {
    const result = calculateSizing({
        max_concurrently_connected_nodes: Number(args.max_concurrently_connected_nodes),
        metrics_per_node: args.metrics_per_node !== undefined ? Number(args.metrics_per_node) : undefined,
        ephemerality: args.ephemerality !== undefined ? Number(args.ephemerality) : undefined,
        clustered_parent: args.clustered_parent !== undefined ? Boolean(args.clustered_parent) : undefined,
        ml_enabled: args.ml_enabled !== undefined ? Boolean(args.ml_enabled) : undefined,
        mem_safety_margin: args.mem_safety_margin !== undefined ? Number(args.mem_safety_margin) : undefined,
        cpu_safety_margin: args.cpu_safety_margin !== undefined ? Number(args.cpu_safety_margin) : undefined
    });
    return formatResponse(result);
}

// -------------------------- MCP Tool Definition --------------------------
const tools = [
    {
        name: 'calculate_parent_sizing',
        description: `Calculates the required CPU and RAM for a Netdata Parent node based on infrastructure size and configuration.

<usage>
- Only max_concurrently_connected_nodes is required
- All other parameters have sensible defaults
- Returns breakdown and final recommendations with safety margins
</usage>

<parameters>
Required:
- max_concurrently_connected_nodes: Number of nodes monitored at peak time

Optional (with defaults):
- metrics_per_node: Metrics/s per node (default: 5000)
- ephemerality: Stability factor, 1=stable, 10=highly ephemeral (default: 2)
- clustered_parent: Whether parents are clustered (default: true)
- ml_enabled: Machine Learning enabled (default: true)
- mem_safety_margin: Memory headroom (default: 0.30 = 30%)
- cpu_safety_margin: CPU headroom for queries (default: 0.40 = 40%)
</parameters>

<examples>
Minimal (just node count, uses all defaults):
{ "max_concurrently_connected_nodes": 100 }

Custom configuration:
{ "max_concurrently_connected_nodes": 500, "metrics_per_node": 10000, "ephemerality": 5 }
</examples>`,
        inputSchema: {
            type: 'object',
            properties: {
                max_concurrently_connected_nodes: {
                    type: 'number',
                    description: 'Number of concurrently monitored nodes, at peak time (required)'
                },
                metrics_per_node: {
                    type: 'number',
                    description: 'Number of metrics/s per node. Default: 5000'
                },
                ephemerality: {
                    type: 'number',
                    description: 'Ephemerality factor (1 = stable infra, 10 = k8s clusters autoscaling daily). Default: 2'
                },
                clustered_parent: {
                    type: 'boolean',
                    description: 'Whether parents are clustered (stream to each other). Default: true'
                },
                ml_enabled: {
                    type: 'boolean',
                    description: 'Whether Machine Learning is enabled. Default: true'
                },
                mem_safety_margin: {
                    type: 'number',
                    description: 'Memory to keep free (percentage as decimal, e.g., 0.3 for 30%). Default: 0.3'
                },
                cpu_safety_margin: {
                    type: 'number',
                    description: 'CPU utilization available for queries (percentage as decimal). Default: 0.4'
                }
            },
            required: ['max_concurrently_connected_nodes']
        },
        handler: toolCalculateParentSizing
    }
];

function listToolsResponse() {
    return {
        tools: tools.map((t) => ({
            name: t.name,
            description: t.description,
            inputSchema: t.inputSchema
        }))
    };
}

// -------------------------- Message loop (same pattern as other MCP servers) --------------------------
let buffer = Buffer.alloc(0);

STDIN.on('data', (chunk) => {
    buffer = Buffer.concat([buffer, chunk]);
    processBuffer();
});

STDIN.on('end', () => {
    processBuffer();
    process.exit(0);
});

function processNewlineJSON() {
    while (true) {
        const nl = buffer.indexOf(0x0A); // '\n'
        if (nl === -1) break;

        const lineBuf = buffer.subarray(0, nl);
        buffer = buffer.subarray(nl + 1);

        let lineStr = lineBuf.toString('utf8');
        if (lineStr.endsWith('\r')) {
            lineStr = lineStr.slice(0, -1);
        }

        if (lineStr.length === 0) continue;

        try {
            const msg = JSON.parse(lineStr);
            handleMessage(msg);
        } catch (e) {
            console.error('MCP: Failed to parse NDJSON line:', e.message, 'Line:', lineStr.slice(0, 200));
        }
    }
}

function processBuffer() {
    const bufStr = buffer.toString('utf8');

    if (bufStr.match(/^Content-Length:/i) || bufStr.includes('\r\n\r\n') || bufStr.includes('\n\n')) {
        // Process as LSP-framed messages (handled below)
    } else if (buffer.indexOf(0x0A) !== -1) {
        processNewlineJSON();
        return;
    } else {
        return;
    }

    while (true) {
        const headerEnd = buffer.indexOf('\r\n\r\n');
        if (headerEnd === -1) {
            const headerEndLF = buffer.indexOf('\n\n');
            if (headerEndLF === -1) {
                if (buffer.length > 0 && buffer.length < 8192) {
                    const preview = buffer.subarray(0, Math.min(100, buffer.length)).toString('utf8').replace(/[\r\n]/g, '\\n');
                    console.error('MCP: Waiting for header, buffer has:', buffer.length, 'bytes, preview:', preview);
                }
                return;
            }
            const header = buffer.subarray(0, headerEndLF).toString('utf8');
            const match = /Content-Length:\s*(\d+)/i.exec(header);
            if (!match) {
                console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
                buffer = buffer.subarray(headerEndLF + 2);
                continue;
            }
            const length = parseInt(match[1], 10);
            const total = headerEndLF + 2 + length;
            if (buffer.length < total) return;
            const body = buffer.subarray(headerEndLF + 2, total).toString('utf8');
            buffer = buffer.subarray(total);
            let msg;
            try {
                msg = JSON.parse(body);
            } catch (e) {
                console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
                continue;
            }
            handleMessage(msg);
            continue;
        }
        const sepLen = 4;

        const header = buffer.subarray(0, headerEnd).toString('utf8');
        const match = /Content-Length:\s*(\d+)/i.exec(header);
        if (!match) {
            console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
            buffer = buffer.subarray(headerEnd + sepLen);
            continue;
        }
        const length = parseInt(match[1], 10);
        const total = headerEnd + sepLen + length;
        if (buffer.length < total) return;
        const body = buffer.subarray(headerEnd + sepLen, total).toString('utf8');
        buffer = buffer.subarray(total);
        let msg;
        try {
            msg = JSON.parse(body);
        } catch (e) {
            console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
            continue;
        }
        handleMessage(msg);
    }
}

function handleMessage(msg) {
    const { id, method, params } = msg;

    if (method === 'initialize') {
        const result = {
            protocolVersion: '2024-11-05',
            serverInfo: { name: 'netdata-parents-sizing-mcp', version: '1.0.0' },
            capabilities: { tools: {} },
            instructions: SERVER_INSTRUCTIONS
        };
        respond(id, result);
        return;
    }

    if (method === 'tools/list') {
        respond(id, listToolsResponse());
        return;
    }

    if (method === 'tools/call') {
        const { name, arguments: args } = params || {};
        const tool = tools.find((t) => t.name === name);
        if (!tool) {
            respondError(id, -32601, `Unknown tool: ${name}`);
            return;
        }
        (async () => {
            try {
                const result = await tool.handler(args || {});
                const text = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
                respond(id, { content: [{ type: 'text', text }] });
            } catch (e) {
                const message = e && typeof e.message === 'string' ? e.message : 'Tool error';
                respondError(id, -32000, message);
            }
        })();
        return;
    }

    if (method === 'notifications/initialized') return;
    if (method === 'notifications/cancelled') return;
    if (method === 'prompts/list') { respond(id, { prompts: [] }); return; }
    if (method === 'prompts/get') { respond(id, { prompt: null }); return; }
    if (method === 'resources/list') { respond(id, { resources: [] }); return; }
    if (method === 'resources/read') { respond(id, { contents: [] }); return; }
    if (method === 'logging/setLevel') { respond(id, {}); return; }
    if (method === 'ping') { respond(id, {}); return; }

    console.error('MCP: Unknown method:', String(method), 'id:', id, 'params:', JSON.stringify(params));
    if (id !== undefined) respondError(id, -32601, `Unknown method: ${String(method)}`);
}

STDIN.resume();

// Export for unit testing
export { calculateSizing };
