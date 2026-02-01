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
const SERVER_INSTRUCTIONS = `Netdata Parents Sizing Calculator - Calculate CPU, RAM, and Disk requirements for Netdata Parent nodes.

INPUTS:
- max_concurrently_connected_nodes (required): Number of concurrently monitored nodes at peak time
- metrics_per_node: Metrics per second per node (default: 5000)
- ephemerality: Infrastructure stability factor (default: 2)
- clustered_parent: Whether parents are clustered (default: true)
- ml_enabled: Machine Learning enabled (default: true)
- mem_safety_margin: Memory headroom percentage (default: 0.30 = 30%)
- cpu_safety_margin: CPU headroom for queries percentage (default: 0.40 = 40%)
- days_retention_tier0: Per-second data retention in days (default: 14)
- days_retention_tier1: Per-minute data retention in days (default: 90)
- days_retention_tier2: Per-hour data retention in days (default: 365)

GUIDELINES:
- Simple nodes (bare metal, VMs): ~3000 metrics/s
- Fat Kubernetes nodes: ~10000 metrics/s
- Stable infrastructure: ephemerality = 1
- Daily autoscaling k8s: ephemerality = 10
- Clustered parents stream to each other, requiring more resources

EPHEMERALITY MODEL:
- 1.0 = stable infrastructure, no metric rotation
- 2.0 = all metrics rotate once over the maximum retention period
- Higher values = faster rotation (e.g., 10 = metrics rotate 9x over retention)

OUTPUT:
Returns breakdown of metrics, raw resource needs, disk requirements per tier, and final recommendations with safety margins applied.`;

// -------------------------- Sizing Logic (from Netdata Parents Sizing Guide) --------------------------

// -------------------------- Disk Sizing Constants --------------------------
// Derived from production Netdata deployments (Kubernetes and stable infrastructure)
const DISK_CONSTANTS = {
    // Bytes per sample (after compression)
    // Tier 0: Gorilla compression + ZSTD = excellent compression for time-series
    // Tier 1 & 2: ZSTD only = good compression but includes aggregation overhead
    BYTES_PER_SAMPLE_TIER0: 0.6,   // Gorilla + ZSTD (observed: 0.4-0.7)
    BYTES_PER_SAMPLE_TIER1: 4,     // ZSTD only (observed: 3.9-5.4)
    BYTES_PER_SAMPLE_TIER2: 22,    // ZSTD + fragmentation overhead (observed: 22-25)

    // Samples per day at each granularity
    SAMPLES_PER_DAY_TIER0: 86400,  // 1 sample per second
    SAMPLES_PER_DAY_TIER1: 1440,   // 1 sample per minute
    SAMPLES_PER_DAY_TIER2: 24      // 1 sample per hour
};

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
        cpu_safety_margin = 0.40,
        days_retention_tier0 = 14,
        days_retention_tier1 = 90,
        days_retention_tier2 = 365
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
    if (typeof days_retention_tier0 !== 'number' || days_retention_tier0 < 1) {
        throw new Error('days_retention_tier0 must be a number >= 1');
    }
    if (typeof days_retention_tier1 !== 'number' || days_retention_tier1 < 1) {
        throw new Error('days_retention_tier1 must be a number >= 1');
    }
    if (typeof days_retention_tier2 !== 'number' || days_retention_tier2 < 1) {
        throw new Error('days_retention_tier2 must be a number >= 1');
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

    // --- Disk Sizing Calculations ---
    // Ephemerality model: rotation_rate = (ephemerality - 1) / max_retention_days
    // At ephemerality=1: no rotation. At ephemerality=2: all metrics rotate once over max retention.
    const max_retention_days = Math.max(days_retention_tier0, days_retention_tier1, days_retention_tier2);
    const rotation_rate = (ephemerality - 1) / max_retention_days;

    // Base metrics (currently collected)
    const base_metrics = metrics_curr_collected;

    // Total unique metrics per tier (accounting for rotation over each tier's retention period)
    const metrics_tier0 = base_metrics * (1 + rotation_rate * days_retention_tier0);
    const metrics_tier1 = base_metrics * (1 + rotation_rate * days_retention_tier1);
    const metrics_tier2 = base_metrics * (1 + rotation_rate * days_retention_tier2);

    // Samples per tier
    const samples_tier0 = metrics_tier0 * days_retention_tier0 * DISK_CONSTANTS.SAMPLES_PER_DAY_TIER0;
    const samples_tier1 = metrics_tier1 * days_retention_tier1 * DISK_CONSTANTS.SAMPLES_PER_DAY_TIER1;
    const samples_tier2 = metrics_tier2 * days_retention_tier2 * DISK_CONSTANTS.SAMPLES_PER_DAY_TIER2;

    // Disk per tier (bytes -> GiB)
    const GiB = 1024 ** 3;
    const disk_tier0_gib = (samples_tier0 * DISK_CONSTANTS.BYTES_PER_SAMPLE_TIER0) / GiB;
    const disk_tier1_gib = (samples_tier1 * DISK_CONSTANTS.BYTES_PER_SAMPLE_TIER1) / GiB;
    const disk_tier2_gib = (samples_tier2 * DISK_CONSTANTS.BYTES_PER_SAMPLE_TIER2) / GiB;
    const disk_total_gib = disk_tier0_gib + disk_tier1_gib + disk_tier2_gib;

    // Round disk to multiples of 10 GiB for practical provisioning
    const roundDisk = (n) => Math.max(10, Math.ceil(n / 10) * 10);

    return {
        inputs: {
            max_concurrently_connected_nodes,
            metrics_per_node,
            ephemerality,
            clustered_parent,
            ml_enabled,
            mem_safety_margin,
            cpu_safety_margin,
            days_retention_tier0,
            days_retention_tier1,
            days_retention_tier2
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
        disk: {
            tier0: {
                retention_days: days_retention_tier0,
                granularity: '1s',
                unique_metrics: Math.round(metrics_tier0),
                samples: Math.round(samples_tier0),
                disk_gib: parseFloat(disk_tier0_gib.toFixed(2))
            },
            tier1: {
                retention_days: days_retention_tier1,
                granularity: '1m',
                unique_metrics: Math.round(metrics_tier1),
                samples: Math.round(samples_tier1),
                disk_gib: parseFloat(disk_tier1_gib.toFixed(2))
            },
            tier2: {
                retention_days: days_retention_tier2,
                granularity: '1h',
                unique_metrics: Math.round(metrics_tier2),
                samples: Math.round(samples_tier2),
                disk_gib: parseFloat(disk_tier2_gib.toFixed(2))
            },
            total_gib: parseFloat(disk_total_gib.toFixed(2)),
            total_gib_rounded: roundDisk(disk_total_gib)
        },
        recommendation: {
            cpu_cores: roundCpu(final_cpu_cores),
            ram_gib: roundRam(final_mem_gib),
            disk_gib: roundDisk(disk_total_gib),
            cpu_cores_exact: parseFloat(final_cpu_cores.toFixed(2)),
            ram_gib_exact: parseFloat(final_mem_gib.toFixed(2)),
            disk_gib_exact: parseFloat(disk_total_gib.toFixed(2)),
            details: `Recommended: ${roundCpu(final_cpu_cores)} CPU Cores, ${roundRam(final_mem_gib)} GiB RAM, ${roundDisk(disk_total_gib)} GiB Disk`
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
        cpu_safety_margin,
        days_retention_tier0,
        days_retention_tier1,
        days_retention_tier2
    } = result.inputs;

    const {
        total_metrics_per_second,
        metrics_retention,
        nodes_retention,
        raw_memory_needed_mib,
        raw_cpu_needed_cores
    } = result.breakdown;

    const { tier0, tier1, tier2, total_gib, total_gib_rounded } = result.disk;

    const cpu_cores = result.recommendation.cpu_cores;
    const ram_gib = result.recommendation.ram_gib;
    const disk_gib = result.recommendation.disk_gib;
    const mem_margin_pct = Math.round(mem_safety_margin * 100);
    const cpu_margin_pct = Math.round(cpu_safety_margin * 100);

    // Calculate actual free percentages after rounding
    const raw_ram_gib = raw_memory_needed_mib / 1024;
    const actual_cpu_free_pct = Math.round((1 - raw_cpu_needed_cores / cpu_cores) * 100);
    const actual_ram_free_pct = Math.round((1 - raw_ram_gib / ram_gib) * 100);

    // Format retention as human-readable
    const formatRetention = (days) => {
        if (days >= 365) return `${Math.round(days / 365 * 10) / 10}y`;
        if (days >= 30) return `${Math.round(days / 30 * 10) / 10}mo`;
        return `${days}d`;
    };

    return `# Netdata Parent Sizing: ${max_concurrently_connected_nodes} Nodes

## Configuration Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| **Concurrent Nodes** | ${max_concurrently_connected_nodes} | Distinct monitored hosts (physical servers, VMs, SNMP devices, vnodes). Containers are not counted as nodes. |
| **Metrics per Node** | ${metrics_per_node} | Average metrics/second per node across all collection types. Reference values: bare-metal/VMs ~5k, Kubernetes nodes ~20k, SNMP devices ~300. |
| **Ephemerality** | ${ephemerality}x | Infrastructure rotation factor. 1 = static (no rotation), 2 = all metrics rotate once over retention period, 10 = 9x rotation. |
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
| **Disk** | ${disk_gib} GiB |

*Included free resources: ${actual_cpu_free_pct}% CPU, ${actual_ram_free_pct}% RAM.*

## Disk Requirements by Tier

| Tier | Granularity | Retention | Unique Metrics | Samples | Disk Size |
|------|-------------|-----------|----------------|---------|-----------|
| **0** | per-second | ${formatRetention(days_retention_tier0)} | ${formatNumber(tier0.unique_metrics)} | ${formatNumber(tier0.samples)} | ${tier0.disk_gib} GiB |
| **1** | per-minute | ${formatRetention(days_retention_tier1)} | ${formatNumber(tier1.unique_metrics)} | ${formatNumber(tier1.samples)} | ${tier1.disk_gib} GiB |
| **2** | per-hour | ${formatRetention(days_retention_tier2)} | ${formatNumber(tier2.unique_metrics)} | ${formatNumber(tier2.samples)} | ${tier2.disk_gib} GiB |

**Total Disk: ${total_gib} GiB** (rounded to ${total_gib_rounded} GiB for provisioning)

### Disk Calculation Assumptions

**Compression (bytes per sample on disk):**

| Tier | Compression | Bytes/Sample | Rationale |
|------|-------------|--------------|-----------|
| **0** | Gorilla + ZSTD | 0.6 | Time-series optimized compression, excellent for per-second data |
| **1** | ZSTD | 4 | Standard compression for aggregated per-minute data |
| **2** | ZSTD | 22 | Includes fragmentation overhead from sparse/ephemeral metrics |

**Ephemerality Model:**

This calculation assumes metrics rotate **uniformly** over the maximum retention period (${formatRetention(Math.max(days_retention_tier0, days_retention_tier1, days_retention_tier2))}).

- **Ephemerality = ${ephemerality}** means all currently collected metrics will be replaced **${(ephemerality - 1).toFixed(1)} times** over ${formatRetention(Math.max(days_retention_tier0, days_retention_tier1, days_retention_tier2))}
- **Daily rotation rate:** ${((ephemerality - 1) / Math.max(days_retention_tier0, days_retention_tier1, days_retention_tier2) * 100).toFixed(2)}% of metrics rotate per day
- **Unique metrics per tier** = base metrics × (1 + rotation_rate × tier_retention_days)

| Tier | Retention | Metric Growth | Unique Metrics |
|------|-----------|---------------|----------------|
| **0** | ${formatRetention(days_retention_tier0)} | +${((tier0.unique_metrics / total_metrics_per_second - 1) * 100).toFixed(1)}% | ${formatNumber(tier0.unique_metrics)} |
| **1** | ${formatRetention(days_retention_tier1)} | +${((tier1.unique_metrics / total_metrics_per_second - 1) * 100).toFixed(1)}% | ${formatNumber(tier1.unique_metrics)} |
| **2** | ${formatRetention(days_retention_tier2)} | +${((tier2.unique_metrics / total_metrics_per_second - 1) * 100).toFixed(1)}% | ${formatNumber(tier2.unique_metrics)} |

*Note: Actual disk usage may vary based on metric value patterns, compression efficiency, and rotation timing.*

---

### Rounding Rules
- CPU cores: rounded up to nearest even number (2, 4, 6, 8...)
- RAM: rounded up to multiples of 4 GiB, minimum 4 GiB
- Disk: rounded up to multiples of 10 GiB, minimum 10 GiB

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
        cpu_safety_margin: args.cpu_safety_margin !== undefined ? Number(args.cpu_safety_margin) : undefined,
        days_retention_tier0: args.days_retention_tier0 !== undefined ? Number(args.days_retention_tier0) : undefined,
        days_retention_tier1: args.days_retention_tier1 !== undefined ? Number(args.days_retention_tier1) : undefined,
        days_retention_tier2: args.days_retention_tier2 !== undefined ? Number(args.days_retention_tier2) : undefined
    });
    return formatResponse(result);
}

// -------------------------- MCP Tool Definition --------------------------
const tools = [
    {
        name: 'calculate_parent_sizing',
        description: `Calculates the required CPU, RAM, and Disk for a Netdata Parent node based on infrastructure size and configuration.

<usage>
- Only max_concurrently_connected_nodes is required
- All other parameters have sensible defaults
- Returns breakdown and final recommendations with safety margins
- Includes detailed disk requirements per tier with assumptions
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
- days_retention_tier0: Per-second data retention in days (default: 14)
- days_retention_tier1: Per-minute data retention in days (default: 90)
- days_retention_tier2: Per-hour data retention in days (default: 365)
</parameters>

<examples>
Minimal (just node count, uses all defaults):
{ "max_concurrently_connected_nodes": 100 }

Custom configuration:
{ "max_concurrently_connected_nodes": 500, "metrics_per_node": 10000, "ephemerality": 5 }

With custom retention:
{ "max_concurrently_connected_nodes": 100, "days_retention_tier0": 7, "days_retention_tier1": 30, "days_retention_tier2": 180 }
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
                },
                days_retention_tier0: {
                    type: 'number',
                    description: 'Per-second data retention in days. Default: 14'
                },
                days_retention_tier1: {
                    type: 'number',
                    description: 'Per-minute data retention in days. Default: 90'
                },
                days_retention_tier2: {
                    type: 'number',
                    description: 'Per-hour data retention in days. Default: 365'
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
