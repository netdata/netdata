# Verify local collector contexts and dimensions

## Question

How can an assistant verify that a local Netdata Agent is collecting a
new or changed collector under the expected metric contexts and
dimensions, and that obsolete contexts are absent?

## Inputs

- Local agent host, usually `127.0.0.1:19999`.
- `NODE_UUID` and `MACHINE_GUID` for that local agent.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- The expected context names and, when checking a regression, the
  obsolete context names that must not be live.

## Steps

1. Load the token-safe direct-agent wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   AUDIT_DIR="$(agents_audit_dir)"
   ```

2. Fetch the current context index:

   ```bash
   agents_query_agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST:19999" \
     --machine-guid "$MACHINE_GUID" \
     GET /api/v3/contexts \
     > "$AUDIT_DIR/contexts.json"
   ```

3. Check that expected contexts are present and obsolete contexts are
   absent:

   ```bash
   jq '{
     expected: {
       "system.hw.sensor.fan.input": (.contexts["system.hw.sensor.fan.input"] // null),
       "system.hw.sensor.fan.alarm": (.contexts["system.hw.sensor.fan.alarm"] // null)
     },
     obsolete: {
       "system.fan_requested_speed": (.contexts["system.fan_requested_speed"] // null),
       "system.fan_state": (.contexts["system.fan_state"] // null)
     }
   }' "$AUDIT_DIR/contexts.json"
   ```

4. Fetch chart definitions to verify dimensions and chart placement:

   ```bash
   agents_query_agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST:19999" \
     --machine-guid "$MACHINE_GUID" \
     GET /api/v1/charts \
     > "$AUDIT_DIR/charts.json"

   jq '[
     .charts
     | to_entries[]
     | select(.value.context | IN("system.hw.sensor.fan.input", "system.hw.sensor.fan.alarm"))
     | {
         chart: .key,
         type: .value.type,
         family: .value.family,
         context: .value.context,
         units: .value.units,
         dimensions: (.value.dimensions | keys)
       }
   ]' "$AUDIT_DIR/charts.json"
   ```

5. Query recent data for a collected context:

   ```bash
   read -r -d '' BODY <<'JSON'
   {
     "scope": {"contexts": ["system.hw.sensor.fan.alarm"]},
     "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
     "window": {"after": -300, "before": 0, "points": 5},
     "aggregations": {
       "metrics": [{"group_by": ["dimension"], "aggregation": "sum"}],
       "time": {"time_group": "average"}
     },
     "format": "json2",
     "options": ["jsonwrap", "minify", "unaligned"],
     "timeout": 30000
   }
   JSON

   agents_query_agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST:19999" \
     --machine-guid "$MACHINE_GUID" \
     POST /api/v3/data "$BODY" \
     > "$AUDIT_DIR/fan-alarm-data.json"

   jq '{dimensions: .view.dimensions.names, points: (.result.data | length)}' \
     "$AUDIT_DIR/fan-alarm-data.json"
   ```

## Output

Expected success shape for the chart check:

```json
[
  {
    "context": "system.hw.sensor.fan.alarm",
    "type": "sensors",
    "family": "Fan",
    "units": "status",
    "dimensions": ["clear", "fault"]
  }
]
```

The obsolete contexts should return `null`. If the expected context is
absent but the installed binary or plugin contains the new strings, the
running service probably has not been restarted or the local hardware
does not expose the data needed to create that chart.

## Notes / gotchas

- Use `/api/v3/contexts` for live context presence and `/api/v1/charts`
  for dimensions, type, family, and units.
- A collector can be correctly installed but absent from live contexts
  until the service restarts and the hardware/API source returns at least
  one collectible instance.
- Store raw responses only under `.local/audits/query-netdata-agents/`;
  do not paste node ids, machine GUIDs, claim ids, or raw bearer tokens
  into durable files.

## Source guides

- [Direct-agent skill](../SKILL.md)
- [Direct metric queries](../query-metrics.md)
