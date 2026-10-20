"""Vendored snapshot of the Netdata Agent's /mcp tool surface.

Captured from a live agent's tools/list (see scripts/snapshot_agent_tools.py).
These are the agent's OWN tool name -> {description, inputSchema}; the build-MCP
wrapper re-exposes each as `netdata_agent_<name>` with an injected `agent_id` and
forwards to the agent's /mcp. Drift from the live agent is accepted (a pinned
harness surface); refresh by re-running the snapshot script.
"""

from __future__ import annotations

from typing import Any

AGENT_TOOLS: dict[str, dict[str, Any]] = {
    "execute_function": {
        "description": "Executes live data collection functions on nodes. Common functions: 'processes' (running processes), 'network-connections' (active connections), 'mount-points' (disk mounts), 'systemd-services' (service status). Returns tabular data with filtering and sorting options.\n",
        "inputSchema": {
            "type": "object",
            "title": "Execute a function on a specific node. Functions provide live information and they are automatically routed and executed to Netdata running on the given node.",
            "properties": {
                "node": {
                    "type": "string",
                    "title": "The node on which to execute the function",
                    "description": "The hostname or machine_guid or node_id of the node where the function should be executed. The node needs to be online (live) and reachable."
                },
                "function": {
                    "type": "string",
                    "title": "The name of the function to execute.",
                    "description": "The function name, as available in the node_details tool output"
                },
                "timeout": {
                    "type": "number",
                    "title": "Execution timeout in seconds",
                    "description": "Maximum time to wait for function execution (default: 60)",
                    "default": 60,
                    "minimum": 1,
                    "maximum": 3600
                },
                "columns": {
                    "type": "array",
                    "title": "Columns to include",
                    "description": "Array of column names to include in the result. Each function has its own columns, so first check the function without this parameter.",
                    "items": {
                        "type": "string"
                    }
                },
                "sort_column": {
                    "type": "string",
                    "title": "Column to sort by",
                    "description": "Name of the column to sort the results by."
                },
                "sort_order": {
                    "type": "string",
                    "title": "Sort order",
                    "description": "Order to sort results: 'asc' for ascending, 'desc' for descending",
                    "default": "desc",
                    "enum": [
                        "asc",
                        "desc"
                    ]
                },
                "limit": {
                    "type": "number",
                    "title": "Limit",
                    "description": "Number of entries to return",
                    "default": 0
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for query window.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for query window.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cursor": {
                    "type": "string",
                    "title": "Pagination cursor",
                    "description": "Opaque cursor for pagination (follows MCP standard)"
                },
                "direction": {
                    "type": "string",
                    "title": "Query direction",
                    "description": "Direction for query processing: 'forward' (oldest first) or 'backward' (newest first)",
                    "default": "backward",
                    "enum": [
                        "forward",
                        "backward"
                    ]
                },
                "q": {
                    "type": "string",
                    "title": "Full-text search",
                    "description": "Full-text search to filter results. Use pipe character (|) to separate multiple search patterns. Example: '*fail*|*error*|*systemd*'. Wildcards (*) are supported for pattern matching."
                },
                "conditions": {
                    "type": "array",
                    "title": "Filter conditions",
                    "description": "Array of conditions to filter rows. Each condition is an array of [column, operator, value] where operator can be ==, !=, <>, <, <=, >, >=, match, not match. Use '*' or '' (empty string) as column name to search across all columns.",
                    "items": {
                        "type": "array",
                        "items": {
                            "oneOf": [
                                {
                                    "type": "string"
                                },
                                {
                                    "type": "string",
                                    "enum": [
                                        "==",
                                        "!=",
                                        "<>",
                                        "<",
                                        "<=",
                                        ">",
                                        ">=",
                                        "match",
                                        "not match"
                                    ]
                                },
                                {
                                    "oneOf": [
                                        {
                                            "type": "string"
                                        },
                                        {
                                            "type": "number"
                                        },
                                        {
                                            "type": "boolean"
                                        }
                                    ]
                                }
                            ]
                        }
                    }
                },
                "selections": {
                    "type": "object",
                    "title": "Function parameter selections",
                    "description": "Key-value pairs where each key is a parameter name and the value depends on the parameter type: for 'select' type parameters, use a single string value; for 'multiselect' type parameters, use an array of strings. Functions that require selections will prompt you with available options when called without this parameter. Example: {\"param1\": \"single_value\", \"param2\": [\"value1\", \"value2\"]}"
                }
            },
            "required": [
                "node",
                "function"
            ]
        }
    },
    "find_anomalous_metrics": {
        "description": "Finds metrics that were behaving anomalously according to Netdata's ML models. Returns metrics ranked by their anomaly rates (0 to 1, representing 0-100% of time anomalous). IMPORTANT: For large infrastructures, use filters (metrics, nodes, instances, dimensions, or labels) to narrow the scope and avoid timeouts.",
        "inputSchema": {
            "type": "object",
            "title": "Find metrics with highest anomaly rates",
            "properties": {
                "after": {
                    "title": "Start time",
                    "description": "Start time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "before": {
                    "title": "End time",
                    "description": "End time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of metrics (contexts) to filter (e.g., ['system.cpu', 'disk.io', 'mysql.queries']). Use 'list_metrics' to discover available metrics.",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of nodes to filter (e.g., ['web-server-1', 'database-primary']). Use 'list_nodes' to discover available nodes.",
                    "items": {
                        "type": "string"
                    }
                },
                "instances": {
                    "type": "array",
                    "title": "Filter by instances",
                    "description": "Array of metric instances to filter (e.g., ['eth0', 'sda', 'production_db']). Use 'get_metrics_details' to discover instances for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "dimensions": {
                    "type": "array",
                    "title": "Filter by dimensions",
                    "description": "Array of dimension names to filter (e.g., ['user', 'writes', 'slow_queries']). Use 'get_metrics_details' to discover dimensions for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "labels": {
                    "type": "object",
                    "title": "Filter by labels",
                    "description": "Filter using labels where each key maps to an array of exact values. Values in the same array are ORed, different keys are ANDed. Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\nNote: Wildcards are not supported. Use exact label keys and values only. Use 'get_metrics_details' to discover available labels.",
                    "additionalProperties": {
                        "type": "array",
                        "items": {
                            "type": "string"
                        }
                    }
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of results to return",
                    "default": 50,
                    "minimum": 30,
                    "maximum": 500
                },
                "timeout": {
                    "type": "number",
                    "title": "Query timeout",
                    "description": "Maximum time to wait for the query to complete (in seconds)",
                    "default": 300,
                    "minimum": 1,
                    "maximum": 3600
                }
            },
            "required": [
                "after",
                "before"
            ]
        }
    },
    "find_correlated_metrics": {
        "description": "Finds metrics that changed significantly during an incident by comparing a problem time period with a normal baseline period. Essential for root cause analysis. IMPORTANT: For large infrastructures, use filters (metrics, nodes, instances, dimensions, or labels) to narrow the scope and avoid timeouts.",
        "inputSchema": {
            "type": "object",
            "title": "Find metrics that changed during an incident",
            "properties": {
                "after": {
                    "title": "Start time",
                    "description": "Start time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "before": {
                    "title": "End time",
                    "description": "End time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "baseline_after": {
                    "title": "Baseline start time",
                    "description": "Start time for the baseline period to compare against. If not specified, automatically set to 4x the query window before the query period.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'baseline_before' (e.g. -3600 for an hour before 'baseline_before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "baseline_before": {
                    "title": "Baseline end time",
                    "description": "End time for the baseline period. If not specified, automatically set to the start of the query period (adjacent to 'after').",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'after' (e.g. -3600 for an hour before 'after'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of metrics (contexts) to filter (e.g., ['system.cpu', 'disk.io', 'mysql.queries']). Use 'list_metrics' to discover available metrics.",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of nodes to filter (e.g., ['web-server-1', 'database-primary']). Use 'list_nodes' to discover available nodes.",
                    "items": {
                        "type": "string"
                    }
                },
                "instances": {
                    "type": "array",
                    "title": "Filter by instances",
                    "description": "Array of metric instances to filter (e.g., ['eth0', 'sda', 'production_db']). Use 'get_metrics_details' to discover instances for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "dimensions": {
                    "type": "array",
                    "title": "Filter by dimensions",
                    "description": "Array of dimension names to filter (e.g., ['user', 'writes', 'slow_queries']). Use 'get_metrics_details' to discover dimensions for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "labels": {
                    "type": "object",
                    "title": "Filter by labels",
                    "description": "Filter using labels where each key maps to an array of exact values. Values in the same array are ORed, different keys are ANDed. Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\nNote: Wildcards are not supported. Use exact label keys and values only. Use 'get_metrics_details' to discover available labels.",
                    "additionalProperties": {
                        "type": "array",
                        "items": {
                            "type": "string"
                        }
                    }
                },
                "method": {
                    "type": "string",
                    "title": "Correlation method",
                    "description": "Algorithm to use:\n- 'ks2': Statistical distribution comparison (slow, but intelligent)\n- 'volume': Percentage change in averages (fast, works well for most cases)",
                    "enum": [
                        "ks2",
                        "volume"
                    ],
                    "default": "volume"
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of results to return",
                    "default": 50,
                    "minimum": 30,
                    "maximum": 500
                },
                "timeout": {
                    "type": "number",
                    "title": "Query timeout",
                    "description": "Maximum time to wait for the query to complete (in seconds)",
                    "default": 300,
                    "minimum": 1,
                    "maximum": 3600
                }
            },
            "required": [
                "after",
                "before"
            ]
        }
    },
    "find_unstable_metrics": {
        "description": "Finds metrics with the highest variability using coefficient of variation (standard deviation as % of mean). Useful for identifying unstable or fluctuating metrics. IMPORTANT: For large infrastructures, use filters (metrics, nodes, instances, dimensions, or labels) to narrow the scope and avoid timeouts.",
        "inputSchema": {
            "type": "object",
            "title": "Find metrics with high variability",
            "properties": {
                "after": {
                    "title": "Start time",
                    "description": "Start time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "before": {
                    "title": "End time",
                    "description": "End time for metrics.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of metrics (contexts) to filter (e.g., ['system.cpu', 'disk.io', 'mysql.queries']). Use 'list_metrics' to discover available metrics.",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of nodes to filter (e.g., ['web-server-1', 'database-primary']). Use 'list_nodes' to discover available nodes.",
                    "items": {
                        "type": "string"
                    }
                },
                "instances": {
                    "type": "array",
                    "title": "Filter by instances",
                    "description": "Array of metric instances to filter (e.g., ['eth0', 'sda', 'production_db']). Use 'get_metrics_details' to discover instances for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "dimensions": {
                    "type": "array",
                    "title": "Filter by dimensions",
                    "description": "Array of dimension names to filter (e.g., ['user', 'writes', 'slow_queries']). Use 'get_metrics_details' to discover dimensions for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "labels": {
                    "type": "object",
                    "title": "Filter by labels",
                    "description": "Filter using labels where each key maps to an array of exact values. Values in the same array are ORed, different keys are ANDed. Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\nNote: Wildcards are not supported. Use exact label keys and values only. Use 'get_metrics_details' to discover available labels.",
                    "additionalProperties": {
                        "type": "array",
                        "items": {
                            "type": "string"
                        }
                    }
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of results to return",
                    "default": 50,
                    "minimum": 30,
                    "maximum": 500
                },
                "timeout": {
                    "type": "number",
                    "title": "Query timeout",
                    "description": "Maximum time to wait for the query to complete (in seconds)",
                    "default": 300,
                    "minimum": 1,
                    "maximum": 3600
                }
            },
            "required": [
                "after",
                "before"
            ]
        }
    },
    "get_metrics_details": {
        "description": "Gets comprehensive metadata for specific metrics. Returns titles, units, dimensions, instances, labels, and collection status.\n",
        "inputSchema": {
            "type": "object",
            "title": "Get metrics details",
            "properties": {
                "metrics": {
                    "type": "array",
                    "title": "Specify the metrics",
                    "description": "Array of specific metric names to retrieve details for. This parameter is required. Each metric must be an exact match - no wildcards or patterns allowed. Examples: [\"system.cpu\", \"system.load\", \"system.ram\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of specific node names to filter by. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. If not specified, all nodes are included. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for metrics with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for metrics with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 50,
                    "minimum": 1,
                    "maximum": 500
                }
            },
            "required": [
                "metrics"
            ]
        }
    },
    "get_nodes_details": {
        "description": "Gets comprehensive node information including hardware specs, OS details, capabilities, health status, available functions, and monitoring configuration. Essential for understanding node capabilities before executing functions.\n",
        "inputSchema": {
            "type": "object",
            "title": "Get detailed information about monitored nodes",
            "properties": {
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of specific metric names to filter by. Each metric must be an exact match - no wildcards or patterns allowed. Use 'list_metrics' to discover available metrics. If not specified, all metrics are included. Examples: [\"system.cpu\", \"system.load\"], [\"disk.io\", \"disk.space\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Specify the nodes",
                    "description": "Array of specific node names to query. This parameter is required because this tool produces detailed output. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for nodes with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for nodes with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 50,
                    "minimum": 1,
                    "maximum": 500
                }
            },
            "required": [
                "nodes"
            ]
        }
    },
    "list_alert_transitions": {
        "description": "List recent alert state transitions showing how alerts changed over time",
        "inputSchema": {
            "type": "object",
            "title": "List alert transitions",
            "properties": {
                "after": {
                    "title": "Start time",
                    "description": "Start time for alert transitions.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for alert transitions.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "alerts": {
                    "type": "array",
                    "title": "Filter by alert names",
                    "description": "Array of specific alert names to filter by. Each alert name must be an exact match - no wildcards or patterns allowed. Use 'list_running_alerts' to discover available alert names. If not specified, all alerts are included. Examples: [\"disk_space_usage\", \"cpu_iowait\", \"ram_in_use\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter nodes",
                    "description": "Show only alerts transitions for these nodes.\nUse 'list_nodes' to discover available nodes.\nIf not specified, alerts transitions from all nodes are included. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of specific metric names to filter by. Each metric must be an exact match - no wildcards or patterns allowed. Use 'list_metrics' to discover available metrics. If not specified, all metrics are included. Examples: [\"system.cpu\", \"system.load\"], [\"disk.io\", \"disk.space\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "instances": {
                    "type": "array",
                    "title": "Filter by instances",
                    "description": "Query only the given instances.\nUse the 'get_metrics_details' tool to discover available instances for a metric.\nIf no instances are specified, all instances of the metric are queried.\nExample: [\"instance1\", \"instance2\", \"instance3\"]\n.IMPORTANT: when you have a choice, prefer to filter by labels instead of instances, because many monitored components may change instance names over time.",
                    "items": {
                        "type": "string"
                    }
                },
                "status": {
                    "type": "array",
                    "title": "Filter by status",
                    "description": "Select the alert statuses of interest. At least one status must be selected.\n - CRITICAL: the highest severity, indicates a critical issue that needs immediate attention.\n - WARNING: indicates a potential issue that should be monitored but is not critical.\n - CLEAR: the normal state state for alerts, indicating that the alert is not triggered.\n - UNDEFINED: the alerts failed to be evaluated (some variable of it is undefined, division by zero, etc).\n - UNINITIALIZED: the alert has not been initialized for the first time yet, no data available.\n - REMOVED: the alert was removed (happens during netdata shutdown, child disconnect, health reload).\nMultiple statuses can be selected. Example: [\"CRITICAL\", \"WARNING\"]",
                    "items": {
                        "type": "string",
                        "enum": [
                            "CRITICAL",
                            "WARNING",
                            "CLEAR",
                            "UNDEFINED",
                            "UNINITIALIZED",
                            "REMOVED"
                        ]
                    }
                },
                "classifications": {
                    "type": "array",
                    "title": "Filter by classifications",
                    "description": "Array of specific alert classifications to filter by. Each classification must be an exact match - no wildcards or patterns allowed. Use 'list_running_alerts' to discover available classifications. If not specified, all classifications are included. Examples: [\"Errors\", \"Latency\", \"Utilization\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "types": {
                    "type": "array",
                    "title": "Filter by types",
                    "description": "Array of specific alert types to filter by. Each type must be an exact match - no wildcards or patterns allowed. Use 'list_running_alerts' to discover available types. If not specified, all types are included. Examples: [\"System\", \"Web Server\", \"Database\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "components": {
                    "type": "array",
                    "title": "Filter by components",
                    "description": "Array of specific components to filter by. Each component must be an exact match - no wildcards or patterns allowed. Use 'list_running_alerts' to discover available components. If not specified, all components are included. Examples: [\"Network\", \"Disk\", \"Memory\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "roles": {
                    "type": "array",
                    "title": "Filter by roles",
                    "description": "Array of specific roles to filter by. Each role must be an exact match - no wildcards or patterns allowed. Use 'list_running_alerts' to discover available roles. If not specified, all roles are included. Examples: [\"sysadmin\", \"webmaster\", \"dba\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Number of most recent alert transitions to return",
                    "default": 100,
                    "minimum": 1,
                    "maximum": 500
                },
                "cursor": {
                    "type": "string",
                    "title": "Pagination cursor",
                    "description": "Pagination cursor from previous response. Use the 'nextCursor' value from the previous response to get the next page of results."
                },
                "timeout": {
                    "type": "number",
                    "title": "Query timeout",
                    "description": "Maximum time to wait for the query to complete (in seconds)",
                    "default": 60,
                    "minimum": 1,
                    "maximum": 3600
                }
            },
            "required": [
                "status"
            ]
        }
    },
    "list_functions": {
        "description": "Lists all available Netdata functions that can be executed on nodes. Returns function names, descriptions, and execution requirements. Use this to discover what functions are available before executing them.\n",
        "inputSchema": {
            "type": "object",
            "title": "List available functions",
            "properties": {
                "nodes": {
                    "type": "array",
                    "title": "Specify the nodes",
                    "description": "Array of specific node names to query. This parameter is required because this tool produces detailed output. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                }
            },
            "required": [
                "nodes"
            ]
        }
    },
    "list_metrics": {
        "description": "Lists available metrics (contexts) with time-aware filtering. Returns metric names matching search patterns, filtered by nodes and time window. Supports full-text search across names, titles, instances, dimensions, and labels.\n",
        "inputSchema": {
            "type": "object",
            "title": "List available metrics",
            "properties": {
                "metrics": {
                    "type": "string",
                    "title": "Filter metrics",
                    "description": "Pattern matching on metric names. Use pipe (|) to separate multiple patterns. Supports wildcards. Examples: 'system.*', '*cpu*|*memory*', 'disk.*|net.*|system.*'",
                    "default": "*"
                },
                "q": {
                    "type": "string",
                    "title": "Full-text search on metrics metadata",
                    "description": "Filter metrics by searching across all their metadata (names, titles, instances, dimensions, labels). Use pipe (|) to separate multiple search terms. Examples: 'memory|pressure', 'cpu|load|system'"
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of specific node names to filter by. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. If not specified, all nodes are included. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for metrics with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for metrics with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 50,
                    "minimum": 1,
                    "maximum": 500
                }
            }
        }
    },
    "list_nodes": {
        "description": "Lists all monitored nodes in the infrastructure. Returns node IDs, hostnames, connection status, and parent-child relationships. Use this to discover available nodes before querying metrics or executing functions.\n",
        "inputSchema": {
            "type": "object",
            "title": "List monitored nodes",
            "properties": {
                "metrics": {
                    "type": "array",
                    "title": "Filter by metrics",
                    "description": "Array of specific metric names to filter by. Each metric must be an exact match - no wildcards or patterns allowed. Use 'list_metrics' to discover available metrics. If not specified, all metrics are included. Examples: [\"system.cpu\", \"system.load\"], [\"disk.io\", \"disk.space\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "string",
                    "title": "Filter nodes",
                    "description": "Search for nodes by hostname patterns. This is the primary way to find specific nodes without retrieving the full list. Use pipe (|) to separate multiple patterns. Wildcards (*) are supported for flexible matching. Examples: 'node1|node2' (exact names), '*web*' (contains 'web'), 'prod-*' (starts with 'prod-'), '*db*|*cache*' (contains 'db' or 'cache')",
                    "default": "*"
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for nodes with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for nodes with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 50,
                    "minimum": 1,
                    "maximum": 500
                }
            }
        }
    },
    "list_raised_alerts": {
        "description": "List currently active alerts (WARNING and CRITICAL status) across all nodes",
        "inputSchema": {
            "type": "object",
            "title": "List raised alerts",
            "properties": {
                "metrics": {
                    "type": "array",
                    "title": "Filter by contexts",
                    "description": "Array of specific context names to filter alerts by. Each context must be an exact match - no wildcards or patterns allowed. Use 'list_metrics' to discover available contexts. If not specified, alerts from all contexts are included. Examples: [\"system.cpu\", \"disk.space\"], [\"mysql.queries\", \"redis.memory\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of specific node names to filter by. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. If not specified, all nodes are included. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 200,
                    "minimum": 1,
                    "maximum": 500
                },
                "alerts": {
                    "type": "string",
                    "title": "Filter alerts",
                    "description": "Pattern matching on alert names. Use pipe (|) to separate multiple patterns. Supports wildcards. Examples: 'disk_*', '*cpu*|*memory*', 'health.*'",
                    "default": "*"
                }
            }
        }
    },
    "list_running_alerts": {
        "description": "List all alerts including cleared, undefined, and uninitialized alerts across all nodes",
        "inputSchema": {
            "type": "object",
            "title": "List all alerts",
            "properties": {
                "metrics": {
                    "type": "array",
                    "title": "Filter by contexts",
                    "description": "Array of specific context names to filter alerts by. Each context must be an exact match - no wildcards or patterns allowed. Use 'list_metrics' to discover available contexts. If not specified, alerts from all contexts are included. Examples: [\"system.cpu\", \"disk.space\"], [\"mysql.queries\", \"redis.memory\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Filter by nodes",
                    "description": "Array of specific node names to filter by. Each node must be an exact match - no wildcards or patterns allowed. Use 'list_nodes' to discover available nodes. If not specified, all nodes are included. Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                    "items": {
                        "type": "string"
                    }
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for alerts with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ],
                    "default": -3600
                },
                "before": {
                    "title": "End time",
                    "description": "End time for alerts with data collected.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Maximum number of items to return per category (dimensions, instances, labels, etc.). Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
                    "default": 200,
                    "minimum": 1,
                    "maximum": 500
                },
                "alerts": {
                    "type": "string",
                    "title": "Filter alerts",
                    "description": "Pattern matching on alert names. Use pipe (|) to separate multiple patterns. Supports wildcards. Examples: 'disk_*', '*cpu*|*memory*', 'health.*'",
                    "default": "*"
                }
            }
        }
    },
    "query_metrics": {
        "description": "Queries time-series metrics data with powerful aggregation options. Specify context, time range, and grouping (by dimension, instance, node, or label). Returns data points with statistics and contribution analysis.\n",
        "inputSchema": {
            "type": "object",
            "title": "Query Metrics Data",
            "properties": {
                "metric": {
                    "type": "string",
                    "title": "Metric Name",
                    "description": "The exact metric (context) to query.\nUse the 'list_metrics' tool to discover available metrics."
                },
                "dimensions": {
                    "type": "array",
                    "title": "Dimensions Filter",
                    "description": "Array of dimensions to include in the query.\nExamples: [\"read\", \"write\"] or [\"in\", \"out\"] or [\"used\", \"free\", \"cached\"]\nUse the 'get_metrics_details' tool to discover the available dimensions for a metric.",
                    "items": {
                        "type": "string"
                    }
                },
                "labels": {
                    "type": "object",
                    "title": "Labels Filter",
                    "description": "Query only the instances with the given labels. Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\nValues in the same array are ORed, different keys are ANDed. Use the 'get_metrics_details' tool to discover available labels and values for a metric.",
                    "additionalProperties": {
                        "type": "array",
                        "items": {
                            "type": "string"
                        }
                    }
                },
                "instances": {
                    "type": "array",
                    "title": "Instances Filter",
                    "description": "Query only the given instances.\nUse the 'get_metrics_details' tool to discover available instances for a metric.\nIf no instances are specified, all instances of the metric are queried.\nExample: [\"instance1\", \"instance2\", \"instance3\"]\n.IMPORTANT: when you have a choice, prefer to filter by labels instead of instances, because many monitored components may change instance names over time.",
                    "items": {
                        "type": "string"
                    }
                },
                "nodes": {
                    "type": "array",
                    "title": "Nodes Filter",
                    "description": "Array of nodes to include in the query.\nIf no nodes are specified, all nodes having data for the given metrics in the specified time-frame will be queried.\nExamples: [\"node1\", \"node2\", \"node3\"]\nUse the 'list_nodes' tool to discover the available nodes.",
                    "items": {
                        "type": "string"
                    }
                },
                "cardinality_limit": {
                    "type": "number",
                    "title": "Cardinality Limit",
                    "description": "Limit the response cardinality (number of dimensions, instances, labels, etc.). When the limit is exceeded, the response will indicate how many items were omitted.",
                    "default": 10,
                    "minimum": 1,
                    "maximum": 500
                },
                "after": {
                    "title": "Start time",
                    "description": "Start time for query window.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to 'before' (e.g. -3600 for an hour before 'before'). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "before": {
                    "title": "End time",
                    "description": "End time for query window.",
                    "anyOf": [
                        {
                            "type": "number",
                            "description": "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to now (e.g. -3600 for an hour before now). NOTE: Use NEGATIVE values for past times."
                        },
                        {
                            "type": "string",
                            "description": "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times."
                        }
                    ]
                },
                "points": {
                    "type": "number",
                    "title": "Data Points",
                    "description": "Number of data points to return.",
                    "default": 60
                },
                "timeout": {
                    "type": "number",
                    "title": "Timeout",
                    "description": "Query timeout in seconds.",
                    "default": 60
                },
                "options": {
                    "type": "string",
                    "title": "Query Options",
                    "description": "Space-separated list of additional query options:\n'percentage': Return values as percentages of total\n'absolute' or 'absolute-sum': Return absolute values for stacked charts\n'display-absolute': Convert percentage values to absolute before application of grouping functions\n'all-dimensions': Include all dimensions, even those with just zero values\nExample: 'absolute percentage'"
                },
                "time_group": {
                    "type": "string",
                    "title": "Time Grouping Method",
                    "description": "Method to group data points over time. The 'extremes' method returns the maximum value for positive numbers and the minimum value for negative numbers, which is particularly useful for showing the highest peaks in both directions on charts.",
                    "default": "average",
                    "enum": [
                        "average",
                        "min",
                        "max",
                        "sum",
                        "incremental-sum",
                        "median",
                        "trimmed-mean",
                        "trimmed-median",
                        "percentile",
                        "stddev",
                        "coefficient-of-variation",
                        "ema",
                        "des",
                        "countif",
                        "extremes"
                    ]
                },
                "time_group_options": {
                    "type": "string",
                    "title": "Time Group Options",
                    "description": "Additional options for time grouping.\nFor 'percentile', specify a percentage (0-100).\nFor 'countif', specify a comparison operator and value (e.g., '>0', '=0', '!=0', '<=10')."
                },
                "tier": {
                    "type": "number",
                    "title": "Storage Tier",
                    "description": "Storage tier to query from.\nIf not specified, Netdata will automatically pick the best tier based on the time-frame and points requested.\nCAUTION: specifying a high-resolution tier (like 0) over long time-frames (like days) may consume significant system resources."
                },
                "group_by": {
                    "type": "array",
                    "title": "Group By",
                    "description": "Specifies how to group metrics across different time-series.\n- 'dimension': Groups by dimension name across all instances/nodes. Example: for disks it provides the aggregate of reads and writes across all disks of all nodes.\n- 'instance': Groups by instance across all nodes. Example: for disks, it provides the aggregate per disk name (sda, sdb, etc), aggregating their reads and writes, across all nodes.\n- 'node': Groups by node. Example: for disks, it provides one metric per node, aggregating reads and writes across all its disks.\n- 'label': Groups by the given label key (use the parameter 'group_by_label' to set the key). Example: for disks, aggregate over key 'disk_type' to get an group all 'physical', 'virtual' and 'partition' separately.\nMultiple groupings can be combined. Example: '[\"dimension\", \"label\"]'.",
                    "default": [
                        "dimension"
                    ],
                    "items": {
                        "type": "string",
                        "enum": [
                            "dimension",
                            "instance",
                            "node",
                            "label"
                        ]
                    }
                },
                "group_by_label": {
                    "type": "string",
                    "title": "Group By Label",
                    "description": "When 'group_by' includes 'label', this parameter specifies the label key to group by.\nExample: if metrics have an 'interface_type' label with values like 'real' or 'virtual', setting 'group_by_label' to 'interface_type' would aggregate metrics separately for physical and virtual network interfaces."
                },
                "aggregation": {
                    "type": "string",
                    "title": "Aggregation Method",
                    "description": "Method to use when aggregating grouped metrics.\n- 'sum': Sum of all grouped metrics (useful for additive metrics like bytes transferred, operations, etc.)\n- 'min': Minimum value among all grouped metrics (useful for finding best performance metrics)\n- 'max': Maximum value among all grouped metrics (useful for finding worst performance metrics, peak resource usage)\n- 'extremes': When values are both positive and negative, shows the maximum value for positive metrics and the minimum value for negative metrics\n- 'average': Average of all grouped metrics (CAUTION: When 'group_by' doesn't include 'dimension', this averages different metric types together - e.g., CPU user + system + idle - which is rarely meaningful)\n- 'percentage': Expresses each grouped metric as a percentage of its group's total (useful for seeing proportional contributions)\n",
                    "enum": [
                        "sum",
                        "min",
                        "max",
                        "extremes",
                        "average",
                        "percentage"
                    ]
                }
            },
            "required": [
                "metric",
                "dimensions",
                "after",
                "before",
                "points",
                "time_group",
                "group_by",
                "aggregation",
                "cardinality_limit"
            ]
        }
    }
}
