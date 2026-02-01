#!/usr/bin/env node
/**
 * Unit tests for Netdata Parents Sizing Calculator.
 * Tests are based on sample data from the Netdata Parents Sizing Guide spreadsheet.
 *
 * Run: node test.js
 */

import { calculateSizing } from './netdata-parents-sizing-mcp-server.js';

console.log("Running Unit Tests for Netdata Parents Sizing Calculator...\n");

const testCases = [
    {
        name: "Minimal input (40 nodes, all defaults)",
        inputs: {
            max_concurrently_connected_nodes: 40
        },
        // Defaults: metrics_per_node=5000, ephemerality=2, clustered=true, ml=true
        // CPU rounds to even, RAM rounds to multiples of 4
        expected: { cpu: 4, ram: 8 }
    },
    {
        name: "Reference case (40 nodes, explicit params)",
        inputs: {
            max_concurrently_connected_nodes: 40,
            metrics_per_node: 5000,
            ephemerality: 2,
            clustered_parent: true,
            ml_enabled: true
        },
        expected: { cpu: 4, ram: 8 }
    },
    {
        name: "0.5M/s Example (250 nodes, 2000 metrics/node)",
        inputs: {
            max_concurrently_connected_nodes: 250,
            metrics_per_node: 2000,
            ephemerality: 2,
            clustered_parent: true,
            ml_enabled: true
        },
        // CPU: 5 -> 6 (even), RAM: 22.1 -> 24 (multiple of 4)
        expected: { cpu: 6, ram: 24 }
    },
    {
        name: "1M/s Example (500 nodes, 2000 metrics/node)",
        inputs: {
            max_concurrently_connected_nodes: 500,
            metrics_per_node: 2000,
            ephemerality: 2,
            clustered_parent: true,
            ml_enabled: true
        },
        // RAM: 44.2 -> 48 (multiple of 4)
        expected: { cpu: 10, ram: 48 }
    },
    {
        name: "10M/s Example (6000 nodes, ~1667 metrics/node)",
        inputs: {
            max_concurrently_connected_nodes: 6000,
            metrics_per_node: 1667,
            ephemerality: 2,
            clustered_parent: true,
            ml_enabled: true
        },
        // CPU: 100.02 -> 102 (even), RAM: 456 -> 456 (already multiple of 4)
        expected: { cpu: 102, ram: 456 }
    },
    {
        name: "Standalone parent (no clustering)",
        inputs: {
            max_concurrently_connected_nodes: 100,
            metrics_per_node: 3000,
            ephemerality: 1,
            clustered_parent: false,
            ml_enabled: true
        },
        // RAM: 10.7 -> 12 (multiple of 4)
        expected: { cpu: 2, ram: 12 }
    },
    {
        name: "ML disabled",
        inputs: {
            max_concurrently_connected_nodes: 100,
            metrics_per_node: 3000,
            ephemerality: 1,
            clustered_parent: true,
            ml_enabled: false
        },
        // RAM: 10.0 -> 12 (multiple of 4)
        expected: { cpu: 2, ram: 12 }
    },
    {
        name: "High Ephemerality (k8s autoscaling)",
        inputs: {
            max_concurrently_connected_nodes: 100,
            metrics_per_node: 5000,
            ephemerality: 10,
            clustered_parent: true,
            ml_enabled: true
        },
        // CPU: 5 -> 6 (even), RAM: 25.3 -> 28 (multiple of 4)
        expected: { cpu: 6, ram: 28 }
    },
    {
        name: "Large k8s environment (500 nodes, 10000 metrics/node)",
        inputs: {
            max_concurrently_connected_nodes: 500,
            metrics_per_node: 10000,
            ephemerality: 5,
            clustered_parent: true,
            ml_enabled: true
        },
        // RAM: 211.4 -> 212 (multiple of 4)
        expected: { cpu: 50, ram: 212 }
    }
];

let passed = 0;
let failed = 0;

testCases.forEach(test => {
    console.log(`Testing: ${test.name}`);
    const result = calculateSizing(test.inputs);

    const cpu = result.recommendation.cpu_cores;
    const ram = result.recommendation.ram_gib;

    const nodes = test.inputs.max_concurrently_connected_nodes;
    const metrics = test.inputs.metrics_per_node ?? 5000;
    const ephemeral = test.inputs.ephemerality ?? 2;
    const clustered = test.inputs.clustered_parent ?? true;
    const ml = test.inputs.ml_enabled ?? true;

    console.log(`  Inputs: Nodes=${nodes}, Metrics/Node=${metrics}, Ephemeral=${ephemeral}, Clustered=${clustered}, ML=${ml}`);
    console.log(`  Breakdown: ${result.breakdown.total_metrics_per_second} metrics/s, Raw CPU=${result.breakdown.raw_cpu_needed_cores}, Raw RAM=${result.breakdown.raw_memory_needed_mib} MiB`);
    console.log(`  Expected: CPU ~${test.expected.cpu}, RAM ~${test.expected.ram} GiB`);
    console.log(`  Actual:   CPU =${cpu}, RAM =${ram} GiB`);

    // Allow tolerance for rounding differences
    const cpuDelta = Math.abs(cpu - test.expected.cpu);
    const ramDelta = Math.abs(ram - test.expected.ram);

    if (cpuDelta <= 2 && ramDelta <= 15) {
        console.log("  \x1b[32m✓ PASS\x1b[0m\n");
        passed++;
    } else {
        console.log("  \x1b[31m✗ FAIL (Deviation too large)\x1b[0m\n");
        failed++;
    }
});

// Test validation
console.log("Testing: Input Validation");

const validationTests = [
    {
        name: "Missing max_concurrently_connected_nodes",
        inputs: {},
        shouldThrow: true
    },
    {
        name: "Negative node count",
        inputs: { max_concurrently_connected_nodes: -1 },
        shouldThrow: true
    },
    {
        name: "Invalid clustered_parent (number instead of boolean)",
        inputs: { max_concurrently_connected_nodes: 40, clustered_parent: 1 },
        shouldThrow: true
    },
    {
        name: "Valid minimal input",
        inputs: { max_concurrently_connected_nodes: 40 },
        shouldThrow: false
    },
    {
        name: "Valid with custom margins",
        inputs: { max_concurrently_connected_nodes: 40, mem_safety_margin: 0.5, cpu_safety_margin: 0.5 },
        shouldThrow: false
    }
];

validationTests.forEach(test => {
    let threw = false;
    try {
        calculateSizing(test.inputs);
    } catch (e) {
        threw = true;
    }

    if (threw === test.shouldThrow) {
        console.log(`  ${test.name}: \x1b[32m✓ PASS\x1b[0m`);
        passed++;
    } else {
        console.log(`  ${test.name}: \x1b[31m✗ FAIL\x1b[0m (Expected throw=${test.shouldThrow}, got throw=${threw})`);
        failed++;
    }
});

// Test defaults are applied correctly
console.log("\nTesting: Default Values");

const defaultsResult = calculateSizing({ max_concurrently_connected_nodes: 100 });
const defaultTests = [
    { name: "metrics_per_node defaults to 5000", check: defaultsResult.inputs.metrics_per_node === 5000 },
    { name: "ephemerality defaults to 2", check: defaultsResult.inputs.ephemerality === 2 },
    { name: "clustered_parent defaults to true", check: defaultsResult.inputs.clustered_parent === true },
    { name: "ml_enabled defaults to true", check: defaultsResult.inputs.ml_enabled === true },
    { name: "mem_safety_margin defaults to 0.3", check: defaultsResult.inputs.mem_safety_margin === 0.3 },
    { name: "cpu_safety_margin defaults to 0.4", check: defaultsResult.inputs.cpu_safety_margin === 0.4 },
    { name: "days_retention_tier0 defaults to 14", check: defaultsResult.inputs.days_retention_tier0 === 14 },
    { name: "days_retention_tier1 defaults to 90", check: defaultsResult.inputs.days_retention_tier1 === 90 },
    { name: "days_retention_tier2 defaults to 365", check: defaultsResult.inputs.days_retention_tier2 === 365 },
];

defaultTests.forEach(test => {
    if (test.check) {
        console.log(`  ${test.name}: \x1b[32m✓ PASS\x1b[0m`);
        passed++;
    } else {
        console.log(`  ${test.name}: \x1b[31m✗ FAIL\x1b[0m`);
        failed++;
    }
});

// Test disk sizing calculations
console.log("\nTesting: Disk Sizing Calculations");

const diskTests = [
    {
        name: "Disk output structure exists",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => result.disk && result.disk.tier0 && result.disk.tier1 && result.disk.tier2 && typeof result.disk.total_gib === 'number'
    },
    {
        name: "Stable infrastructure (ephemerality=1) has minimal metric growth",
        inputs: { max_concurrently_connected_nodes: 100, metrics_per_node: 5000, ephemerality: 1 },
        check: (result) => {
            const base = 100 * 5000; // 500k metrics
            // With ephemerality=1, all tiers should have same unique metrics as base
            return Math.abs(result.disk.tier0.unique_metrics - base) < 1000 &&
                   Math.abs(result.disk.tier1.unique_metrics - base) < 1000 &&
                   Math.abs(result.disk.tier2.unique_metrics - base) < 1000;
        }
    },
    {
        name: "Ephemerality=2 doubles metrics in tier2 (365 days)",
        inputs: { max_concurrently_connected_nodes: 100, metrics_per_node: 5000, ephemerality: 2 },
        check: (result) => {
            const base = 100 * 5000; // 500k metrics
            // rotation_rate = (2-1)/365 = 0.00274
            // tier2_metrics = base * (1 + 0.00274 * 365) = base * 2
            const expected_tier2 = base * 2;
            const actual = result.disk.tier2.unique_metrics;
            const tolerance = expected_tier2 * 0.01; // 1% tolerance
            return Math.abs(actual - expected_tier2) < tolerance;
        }
    },
    {
        name: "Custom retention affects disk calculation",
        inputs: {
            max_concurrently_connected_nodes: 100,
            metrics_per_node: 5000,
            ephemerality: 1,
            days_retention_tier0: 7,
            days_retention_tier1: 30,
            days_retention_tier2: 180
        },
        check: (result) => {
            // With shorter retention, disk should be smaller
            const defaultResult = calculateSizing({
                max_concurrently_connected_nodes: 100,
                metrics_per_node: 5000,
                ephemerality: 1
            });
            return result.disk.total_gib < defaultResult.disk.total_gib;
        }
    },
    {
        name: "Disk recommendation included in output",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => typeof result.recommendation.disk_gib === 'number' && result.recommendation.disk_gib > 0
    },
    {
        name: "Disk rounded to multiples of 10 GiB",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => result.recommendation.disk_gib % 10 === 0
    },
    {
        name: "Tier 0 has correct granularity label",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => result.disk.tier0.granularity === '1s'
    },
    {
        name: "Tier 1 has correct granularity label",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => result.disk.tier1.granularity === '1m'
    },
    {
        name: "Tier 2 has correct granularity label",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => result.disk.tier2.granularity === '1h'
    },
    {
        name: "Total disk is sum of all tiers",
        inputs: { max_concurrently_connected_nodes: 100 },
        check: (result) => {
            const sum = result.disk.tier0.disk_gib + result.disk.tier1.disk_gib + result.disk.tier2.disk_gib;
            return Math.abs(result.disk.total_gib - sum) < 0.01;
        }
    }
];

diskTests.forEach(test => {
    const result = calculateSizing(test.inputs);
    if (test.check(result)) {
        console.log(`  ${test.name}: \x1b[32m✓ PASS\x1b[0m`);
        passed++;
    } else {
        console.log(`  ${test.name}: \x1b[31m✗ FAIL\x1b[0m`);
        console.log(`    Result: ${JSON.stringify(result.disk, null, 2)}`);
        failed++;
    }
});

// Test disk sizing validation
console.log("\nTesting: Disk Retention Validation");

const diskValidationTests = [
    {
        name: "Invalid days_retention_tier0 (0)",
        inputs: { max_concurrently_connected_nodes: 100, days_retention_tier0: 0 },
        shouldThrow: true
    },
    {
        name: "Invalid days_retention_tier1 (negative)",
        inputs: { max_concurrently_connected_nodes: 100, days_retention_tier1: -1 },
        shouldThrow: true
    },
    {
        name: "Valid custom retention",
        inputs: { max_concurrently_connected_nodes: 100, days_retention_tier0: 7, days_retention_tier1: 30, days_retention_tier2: 180 },
        shouldThrow: false
    }
];

diskValidationTests.forEach(test => {
    let threw = false;
    try {
        calculateSizing(test.inputs);
    } catch (e) {
        threw = true;
    }

    if (threw === test.shouldThrow) {
        console.log(`  ${test.name}: \x1b[32m✓ PASS\x1b[0m`);
        passed++;
    } else {
        console.log(`  ${test.name}: \x1b[31m✗ FAIL\x1b[0m (Expected throw=${test.shouldThrow}, got throw=${threw})`);
        failed++;
    }
});

console.log(`\n========================================`);
console.log(`Results: ${passed} passed, ${failed} failed`);
console.log(`========================================`);

process.exit(failed > 0 ? 1 : 0);
