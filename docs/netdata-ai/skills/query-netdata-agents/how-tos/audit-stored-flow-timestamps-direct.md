# Audit stored flow timestamp coverage without exposing raw rows

## Question

Do retained raw NetFlow/IPFIX records contain genuine flow start and end
timestamps that can support duration-aware time-series reconstruction, or only
collector receive-time fallbacks?

## Inputs

- A local Netdata Agent running `netflow-plugin`.
- `NODE_UUID`, `MACHINE_GUID`, and `AGENT_HOST` for the token-safe direct-Agent
  wrapper.
- The Agent cache directory. The system-package default is
  `/var/cache/netdata`; the default raw-flow root is
  `/var/cache/netdata/flows/raw`.

Raw flow rows reveal network endpoints and traffic volumes. This procedure
prints only field names and aggregate counts.

## Steps

1. Confirm the flow Function is available through the token-safe wrapper:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env

   agents_call_function \
       --via agent \
       --node "$NODE_UUID" \
       --host "$AGENT_HOST" \
       --machine-guid "$MACHINE_GUID" \
       --function flows:netflow \
       --body '{"info":true}' \
     | jq '{status, type, has_history}'
   ```

   The public Function intentionally does not expose `FLOW_START_USEC`,
   `FLOW_END_USEC`, or `OBSERVATION_TIME_MILLIS` as grouping or selection
   fields. Inspect their stored presence locally instead of printing Function
   rows.

2. Select one raw-journal instance directory and confirm its size and file
   count:

   ```bash
   CACHE_DIR="${NETDATA_CACHE_DIR:-/var/cache/netdata}"
   RAW_ROOT="$CACHE_DIR/flows/raw"
   RAW_DIR="$(find "$RAW_ROOT" -mindepth 1 -maxdepth 1 -type d -print -quit)"

   test -n "$RAW_DIR"
   du -sh "$RAW_DIR"
   find "$RAW_DIR" -maxdepth 1 -type f -name '*.journal' | wc -l
   ```

   If `RAW_ROOT` contains multiple instance directories, run the remaining
   steps once per directory. Do not merge them silently.

3. Check the exact retained field catalog and protocol values without reading
   row contents:

   ```bash
   journalctl --directory="$RAW_DIR" --no-pager --field=FLOW_VERSION

   journalctl --directory="$RAW_DIR" --no-pager -N \
     | rg 'FLOW_START_USEC|FLOW_END_USEC|OBSERVATION_TIME_MILLIS|_SOURCE_REALTIME_TIMESTAMP' \
     | sort
   ```

   Field-catalog absence means the field does not occur anywhere in the
   retained journal set. Presence proves only that at least one row has it; use
   the next step to estimate coverage.

4. Take a stratified sample from every retained journal file and emit only
   aggregate timestamp checks:

   ```bash
   find "$RAW_DIR" -maxdepth 1 -type f -name '*.journal' -print0 \
     | while IFS= read -r -d '' file; do
         journalctl --file="$file" --no-pager -n 500 -o json 2>/dev/null \
           | jq -s '
               def n($x): (($x // "0") | tonumber);
               reduce .[] as $r
                 ({records:0, start:0, end:0, observation:0,
                   end_eq_source:0, positive_duration:0};
                  .records += 1
                  | .start += (if n($r.FLOW_START_USEC) > 0 then 1 else 0 end)
                  | .end += (if n($r.FLOW_END_USEC) > 0 then 1 else 0 end)
                  | .observation +=
                      (if n($r.OBSERVATION_TIME_MILLIS) > 0 then 1 else 0 end)
                  | .end_eq_source +=
                      (if n($r.FLOW_END_USEC) > 0 and
                          n($r.FLOW_END_USEC) == n($r._SOURCE_REALTIME_TIMESTAMP)
                       then 1 else 0 end)
                  | .positive_duration +=
                      (if n($r.FLOW_START_USEC) > 0 and
                          n($r.FLOW_END_USEC) > n($r.FLOW_START_USEC)
                       then 1 else 0 end))'
       done \
     | jq -s '
         reduce .[] as $x
           ({files_sampled:length, records:0, start:0, end:0, observation:0,
             end_eq_source:0, positive_duration:0};
            .records += $x.records
            | .start += $x.start
            | .end += $x.end
            | .observation += $x.observation
            | .end_eq_source += $x.end_eq_source
            | .positive_duration += $x.positive_duration)'
   ```

5. Interpret the aggregate with Netdata's decoder semantics:

   - `start > 0` and `positive_duration > 0`: at least some records contain a
     usable interval. Validate clock ordering and duration distribution before
     using it for pro-rating.
   - `start == 0`: no sampled record has a usable duration, regardless of
     whether `end` is populated.
   - `end == end_eq_source` together with no start is evidence that the stored
     end is the collector input-time fallback. Netdata fills a missing flow end
     with `input_realtime_usec` before storage in
     `src/crates/netflow-plugin/src/decoder.rs`.
   - `observation > 0` is a separate timestamp and does not by itself provide a
     flow duration.

## Output

The result is a compact aggregate such as:

```json
{
  "files_sampled": 100,
  "records": 50000,
  "start": 0,
  "end": 50000,
  "observation": 0,
  "end_eq_source": 50000,
  "positive_duration": 0
}
```

This example means the retained records do not contain a usable flow interval.
The apparent end timestamp is receive-time fallback data, not proof that the
exporter supplied an end time.

## Notes / gotchas

- The field catalog checks the complete retained journal set; the numeric
  coverage command is deliberately a stratified sample to avoid scanning and
  materializing every sensitive row.
- Equality with `_SOURCE_REALTIME_TIMESTAMP` must be interpreted together with
  the Agent's timestamp-source configuration. With the default `input` source,
  it is collector reception time.
- Never save `journalctl -o json` output for raw flows in a durable or committed
  location. Stream directly into aggregate-only `jq` processing.
- A start/end interval supports only uniform-rate estimation. It cannot recover
  bursts inside the interval.

## Source guides

- [Direct-agent skill](../SKILL.md)
- [Network-flow Functions](../query-flows.md)
- [Validate a local flow Function](./validate-direct-local-flow-function.md)

