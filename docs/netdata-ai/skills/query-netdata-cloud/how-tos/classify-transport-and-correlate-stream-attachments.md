# Classify fleet transport from metric values and correlate stream attachments

## Question

How can an operator identify whether fleet nodes use cellular or Ethernet from a binary metric, build a historical
cellular cohort, and compare that cohort with Netdata Parent stream-attachment events?

This workflow deliberately avoids static host labels. A label describes inventory or configuration; it does not prove
which transport was active during a historical time bucket.

## Inputs

- Cloud space and room IDs, assigned to `SPACE` and `ROOM` after loading the token-safe Cloud wrappers.
- A narrow context pattern for discovering the transport metric, such as `vendor.*wan*`.
- The selected transport context and its cellular/Ethernet dimension names.
- Absolute Unix timestamps `AFTER` and `BEFORE` for the historical window.
- A cohort rule. The examples require at least 12 valid paired hours and at least 90% cellular time.

Raw Cloud responses contain node identifiers. Store them only under `.local/`, which is ignored by the repository.

## Steps

### 1. Discover candidate transport contexts

```bash
source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
agents_load_env
set -euo pipefail

SPACE='YOUR_SPACE_ID'
ROOM='YOUR_ROOM_ID'
AFTER=YOUR_AFTER_UNIX_TIMESTAMP
BEFORE=YOUR_BEFORE_UNIX_TIMESTAMP
CONTEXT_PATTERN='vendor.*wan*'
AUDIT_DIR=".local/audits/query-netdata-cloud/transport-state/$SPACE/$ROOM/$AFTER-$BEFORE"
mkdir -p "$AUDIT_DIR"

DISCOVERY_PAYLOAD=$(jq -nc --arg context "$CONTEXT_PATTERN" '{
  scope: {contexts: [$context]},
  selectors: {nodes: ["*"], contexts: ["*"]}
}')

agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/contexts" "$DISCOVERY_PAYLOAD" \
  > "$AUDIT_DIR/contexts.json"

jq -r '.contexts | keys[]' "$AUDIT_DIR/contexts.json" | sort -u
```

Pick a context whose dimensions represent runtime transport state. Do not select it from its name alone; validate its
values and dimensional relationship in steps 2 and 3.

### 2. Query both transport dimensions by node

Set the exact context, dimensions, and historical window. Hourly buckets preserve the fraction of each hour spent on
each transport while keeping a multi-day response manageable.

```bash
TRANSPORT_CONTEXT='vendor.wan'
CELLULAR_DIMENSION='Cellular'
ETHERNET_DIMENSION='Ethernet'
TRANSPORT_CHUNK_SECONDS=$(( 500 * 3600 ))
TRANSPORT_MANIFEST="$AUDIT_DIR/transport-responses.txt"
: > "$TRANSPORT_MANIFEST"

start=$AFTER
while (( start < BEFORE )); do
  end=$(( start + TRANSPORT_CHUNK_SECONDS ))
  (( end > BEFORE )) && end=$BEFORE
  points=$(( (end - start + 3599) / 3600 ))
  response="$AUDIT_DIR/transport-$start-$end.json"

  TRANSPORT_PAYLOAD=$(jq -nc \
    --arg context "$TRANSPORT_CONTEXT" \
    --arg cellular "$CELLULAR_DIMENSION" \
    --arg ethernet "$ETHERNET_DIMENSION" \
    --argjson after "$start" \
    --argjson before "$end" \
    --argjson points "$points" '{
      scope: {contexts: [$context], dimensions: [$cellular, $ethernet]},
      selectors: {
        nodes: ["*"], contexts: ["*"], instances: ["*"], dimensions: ["*"], labels: ["*"], alerts: ["*"]
      },
      window: {after: $after, before: $before, points: $points},
      aggregations: {
        metrics: [{group_by: ["dimension", "node"], aggregation: "avg"}],
        time: {time_group: "average"}
      },
      format: "json2",
      options: ["jsonwrap", "minify", "unaligned"],
      timeout: 120000
    }')

  agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/data" "$TRANSPORT_PAYLOAD" > "$response"
  printf '%s\n' "$response" >> "$TRANSPORT_MANIFEST"
  start=$end
done

test -s "$TRANSPORT_MANIFEST"
```

The loop preserves the single-request behavior for windows no longer than 500 hours and splits longer windows before
Cloud can clamp an explicit multi-route point target.

### 3. Prove one-hot behavior and build the cellular cohort

The grouped output dimension IDs have the form `dimension,machine-guid`. Convert the response to long form, pair the
two dimensions at each node/timestamp, and require values to stay in `[0,1]` and sum to 1 within a small tolerance.

```bash
while IFS= read -r response; do
  test -s "$response"
  jq -r \
    --arg cellular "$CELLULAR_DIMENSION" \
    --arg ethernet "$ETHERNET_DIMENSION" '
      .view.dimensions.ids as $ids
      | .view.update_every as $bucket_seconds
      | .result.data[] as $row
      | range(0; ($ids | length)) as $i
      | ($ids[$i] | capture("^(?<dimension>[^,]+),(?<node>.+)$")) as $key
      | select($key.dimension == $cellular or $key.dimension == $ethernet)
      | select($row[$i + 1][0] != null)
      | [$key.node, $row[0], $key.dimension, $row[$i + 1][0], ($row[$i + 1][2] // 0), $bucket_seconds]
      | @tsv
    ' "$response"
done < "$TRANSPORT_MANIFEST" \
| sort -k1,1 -k2,2n -k3,3 \
> "$AUDIT_DIR/transport-hourly.tsv"

MIN_VALID_HOURS=12
CELLULAR_THRESHOLD=0.90
COHORT="$AUDIT_DIR/cellular-machine-guids.txt"
: > "$COHORT"

awk -F '\t' -v OFS='\t' \
  -v cellular="$CELLULAR_DIMENSION" \
  -v ethernet="$ETHERNET_DIMENSION" \
  -v min_hours="$MIN_VALID_HOURS" \
  -v threshold="$CELLULAR_THRESHOLD" \
  -v cohort="$COHORT" '
  function abs(v) { return v < 0 ? -v : v }
  {
    pa = $5 + 0
    empty = (pa % 2) >= 1
    partial = (int(pa / 4) % 2) >= 1
    if (empty || partial) {
      excluded_rows++
      next
    }

    key = $1 SUBSEP $2
    if ($3 == cellular) {
      c[key] = $4 + 0
      cb[key] = $6 + 0
    }
    else if ($3 == ethernet) {
      e[key] = $4 + 0
      eb[key] = $6 + 0
    }
  }
  END {
    tolerance = 0.01
    for (key in c) {
      if (!(key in e)) continue
      if (abs(cb[key] - eb[key]) > 0.001) {
        mismatched_buckets++
        continue
      }
      paired++
      split(key, parts, SUBSEP)
      node = parts[1]
      cv = c[key]
      ev = e[key]
      seconds = cb[key]
      paired_seconds += seconds
      if (cv >= -tolerance && cv <= 1 + tolerance && ev >= -tolerance && ev <= 1 + tolerance) in_range++
      if (abs(cv + ev - 1) <= tolerance) complementary++
      if ((abs(cv) <= tolerance || abs(cv - 1) <= tolerance) &&
          (abs(ev) <= tolerance || abs(ev - 1) <= tolerance)) exact_binary++
      if (cv >= -tolerance && cv <= 1 + tolerance &&
          ev >= -tolerance && ev <= 1 + tolerance && abs(cv + ev - 1) <= tolerance) {
        valid_seconds[node] += seconds
        cellular_seconds[node] += cv * seconds
      }
    }
    printf "paired_node_hours\t%.3f\n", paired_seconds / 3600 > "/dev/stderr"
    printf "in_range_ratio\t%.6f\n", paired ? in_range / paired : 0 > "/dev/stderr"
    printf "complementary_ratio\t%.6f\n", paired ? complementary / paired : 0 > "/dev/stderr"
    printf "exact_binary_ratio\t%.6f\n", paired ? exact_binary / paired : 0 > "/dev/stderr"
    printf "excluded_empty_or_partial_rows\t%d\n", excluded_rows > "/dev/stderr"
    printf "mismatched_bucket_pairs\t%d\n", mismatched_buckets > "/dev/stderr"
    for (node in valid_seconds)
      if (valid_seconds[node] >= min_hours * 3600 && cellular_seconds[node] / valid_seconds[node] >= threshold)
        print node > cohort
  }
' "$AUDIT_DIR/transport-hourly.tsv"

sort -u -o "$COHORT" "$COHORT"
wc -l "$COHORT"
```

Evidence is strong when nearly all paired buckets are in range and complementary. `exact_binary_ratio` may be lower
because an hourly average is fractional when a node changes transport during that hour. If the metric is not
complementary, do not use it as a one-hot transport classifier.

### 4. Query Parent-side accepted stream attachments

Parents expose `netdata.streaming.in.reconnects`, dimension `connections`, as an incremental rate. Each per-child chart
has a `machine_guid` label. Query at one-minute resolution in chunks no longer than 480 minutes so Cloud does not clamp
the requested points.

```bash
ATTACHMENT_CONTEXT='netdata.streaming.in.reconnects'
ATTACHMENT_DIMENSION='connections'
CHUNK_SECONDS=28800
ATTACHMENT_MANIFEST="$AUDIT_DIR/attachment-responses.txt"
: > "$ATTACHMENT_MANIFEST"

start=$AFTER
while (( start < BEFORE )); do
  end=$(( start + CHUNK_SECONDS ))
  (( end > BEFORE )) && end=$BEFORE
  points=$(( (end - start + 59) / 60 ))

  ATTACHMENT_PAYLOAD=$(jq -nc \
    --arg context "$ATTACHMENT_CONTEXT" \
    --arg dimension "$ATTACHMENT_DIMENSION" \
    --argjson after "$start" \
    --argjson before "$end" \
    --argjson points "$points" '{
      scope: {contexts: [$context], dimensions: [$dimension]},
      selectors: {
        nodes: ["*"], contexts: ["*"], instances: ["*"], dimensions: ["*"], labels: ["*"], alerts: ["*"]
      },
      window: {after: $after, before: $before, points: $points},
      aggregations: {
        metrics: [{group_by: ["label"], group_by_label: ["machine_guid"], aggregation: "sum"}],
        time: {time_group: "average"}
      },
      format: "json2",
      options: ["jsonwrap", "minify", "unaligned"],
      timeout: 120000
    }')

  response="$AUDIT_DIR/attachments-$start-$end.json"
  agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/data" "$ATTACHMENT_PAYLOAD" > "$response"
  printf '%s\n' "$response" >> "$ATTACHMENT_MANIFEST"
  start=$end
done

test -s "$ATTACHMENT_MANIFEST"
```

### 5. Integrate the rate and join it to the cellular cohort

For each response, multiply each non-null average rate by the response bucket width. Then sum across chunks and retain
only cohort machine GUIDs.

```bash
while IFS= read -r response; do
  test -s "$response"
  jq -r '
    .view.dimensions.ids as $ids
    | .view.update_every as $bucket_seconds
    | .result.data[] as $row
    | range(0; ($ids | length)) as $i
    | select($row[$i + 1][0] != null)
    | ($row[$i + 1][2] // 0) as $pa
    | select(($pa % 2) == 0 and (((($pa / 4) | floor) % 2) == 0))
    | [$ids[$i], ($row[$i + 1][0] * $bucket_seconds)]
    | @tsv
  ' "$response"
done < "$ATTACHMENT_MANIFEST" \
| awk -F '\t' -v OFS='\t' '{attachments[$1] += $2} END {for (node in attachments) print node, attachments[node]}' \
| sort -k1,1 \
> "$AUDIT_DIR/accepted-attachments.tsv"

awk -F '\t' -v OFS='\t' '
  NR == FNR {cohort[$1] = 1; next}
  $1 in cohort {print $1, $2}
' "$COHORT" "$AUDIT_DIR/accepted-attachments.tsv" \
> "$AUDIT_DIR/cellular-accepted-attachments.tsv"
```

The integrated values should be close to integers. Inspect `db.per_tier`, `.view.after`, `.view.before`, and point
annotations when they are not; missing coverage or a coarser tier can make the result unsuitable for exact event counts.

## Output

- A metric-validity summary: paired node-hours, range ratio, complementarity ratio, and exact-binary ratio.
- A machine-GUID cohort that meets explicit minimum-coverage and cellular-share thresholds.
- Accepted Parent attachment counts for cohort members over the same time window.
- Coverage evidence retained in ignored raw response files.

## Notes / gotchas

1. **Attachments are not proven network reconnects.** `netdata.streaming.in.reconnects` observes accepted receiver
   attachments. It does not encode whether the cause was cellular loss, Parent restart, Child restart, configuration,
   failover, or an initial attachment. Call the result `accepted attachments` unless another lifecycle signal proves the
   cause.
2. **Static labels are not historical state.** Inventory labels may be stale, describe capability rather than active
   transport, or change without preserving history.
3. **Use paired dimensions.** A single `Cellular=1` series is weaker evidence than a pair that remains in `[0,1]` and
   sums to 1.
4. **Require coverage.** Nodes with one brief cellular bucket are not a cellular cohort. State both the minimum valid
   hours and the cellular-share threshold.
5. **Parent routes affect semantics.** Summing by child `machine_guid` across Parents counts every accepted attachment.
   A Child intentionally connected to several Parents can contribute events on every route.
6. **Agent/version availability varies.** If `netdata.streaming.in.reconnects` or its `machine_guid` label is absent,
   record that no direct attachment telemetry is available for the requested window; do not substitute a host label or
   infer events from gaps without saying so.

## Source guides

- [query-metrics.md](../query-metrics.md) -- metric scoping, grouping, tier, point, and annotation semantics.
- [fleet-connectivity-slo-queries.md](./fleet-connectivity-slo-queries.md) -- boolean fleet ratios and denominator caveats.
- [SKILL.md](../SKILL.md) -- token-safe wrapper and sensitive-data requirements.
