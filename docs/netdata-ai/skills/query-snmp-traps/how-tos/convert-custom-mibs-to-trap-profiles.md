# Convert custom MIBs into trap profiles

## Question

How can an operator convert device or monitoring-system MIBs into
Netdata SNMP trap profile YAMLs?

## Inputs

- `MIB_DIR`: directory containing the operator-provided MIB files.
- Optional `MIB_MODULE`: one module name to test first, for example
  `NAGIOS-NOTIFY-MIB`.
- `NODE_UUID`: Netdata node UUID, only needed when reloading profiles
  through the Agent Function path.
- `SNMP_TRAPS_JOB`: trap listener job name for verification queries.
  Default examples use `local`.

## Steps

1. Verify the installed helper exists:

   ```bash
   SNMP_TRAP_PROFILE_GEN="${SNMP_TRAP_PROFILE_GEN:-/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen}"

   if ! test -x "$SNMP_TRAP_PROFILE_GEN"; then
     echo "snmp-trap-profile-gen not found at $SNMP_TRAP_PROFILE_GEN" >&2
     echo "Install or upgrade the Netdata go.d plugin package first." >&2
     exit 1
   fi
   ```

2. Convert one MIB module first:

   ```bash
   MIB_DIR="/path/to/vendor-mibs"
   MIB_MODULE="NAGIOS-NOTIFY-MIB"
   OUT_DIR="$(mktemp -d -t snmp-trap-profile-gen.XXXXXX)"

   "$SNMP_TRAP_PROFILE_GEN" generate \
     --source-dir "$MIB_DIR" \
     --mib "$MIB_MODULE" \
     --out-dir "$OUT_DIR"
   ```

3. Review the generated profile before installing it:

   ```bash
   find "$OUT_DIR/profiles" -maxdepth 1 -type f -name '*.yaml' -print
   jq . "$OUT_DIR/profiles/catalogue.json"
   sed -n '1,160p' "$OUT_DIR/profiles/"*.yaml
   ```

4. Convert the full MIB directory after the single-module test works:

   ```bash
   OUT_DIR="$(mktemp -d -t snmp-trap-profile-gen.XXXXXX)"

   "$SNMP_TRAP_PROFILE_GEN" generate \
     --source-dir "$MIB_DIR" \
     --all \
     --out-dir "$OUT_DIR"
   ```

5. Install the generated YAML profiles:

   ```bash
   sudo install -d -o netdata -g netdata -m 0755 /etc/netdata/go.d/snmp.trap-profiles
   sudo install -o netdata -g netdata -m 0644 \
     "$OUT_DIR"/profiles/*.yaml \
     /etc/netdata/go.d/snmp.trap-profiles/
   ```

6. Reload profiles:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env

   NODE_UUID="YOUR_NODE_UUID"

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function snmp_traps:reload-profiles \
     --body '{"info":true}'

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function snmp_traps:reload-profiles \
     --body '{}'
   ```

   If no SNMP trap job is active, the reload action is unavailable by
   design. Re-apply the job through DynCfg or restart the Netdata Agent
   so the next job creation validates and loads the new profile files.

7. Verify that unknown OIDs resolve:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   TRAP_OID="1.3.6.1.4.1.20006.1.6"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --arg oid "$TRAP_OID" '{
     after: -3600,
     before: 0,
     last: 20,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"]
     },
     query: $oid,
     facets: ["TRAP_NAME", "TRAP_CATEGORY", "TRAP_SEVERITY"]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/custom-profile-verify.json

   jq '.facets[]?
       | select((.id // .name) == "TRAP_NAME")
       | .options[]?' \
     .local/audits/query-snmp-traps/custom-profile-verify.json
   ```

## Output

Return whether the helper produced profile YAML, which YAML files were
installed, and whether fresh trap rows now show `TRAP_NAME`,
`TRAP_CATEGORY`, and `TRAP_SEVERITY` from the custom profile. Do not
paste raw MIB text, trap payloads, device IPs, or SNMP credentials into
durable artifacts.

## Notes / gotchas

- The installed Go helper is the supported operator path and has no
  Python or `pysmi` dependency.
- The legacy Python scripts under `tools/snmp-traps-profile-gen/` are
  retained for historical/reference use. Use them only when working
  from the source tree and deliberately testing the old pipeline.
- Helper output is mechanical unless `--classify` is used with an
  OpenAI-compatible endpoint. Review generated category, severity, and
  descriptions before installing profiles for production use.
- If generated YAML is malformed, job creation or profile reload fails
  instead of silently accepting bad profiles.
- A validation run with `NAGIOS-NOTIFY-MIB` produced `nagios.yaml`
  containing four traps: `nHostEvent`, `nHostNotify`, `nSvcEvent`, and
  `nSvcNotify`.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
- [Generic Function invocation through Cloud](../../query-netdata-cloud/query-functions.md)
- [SNMP trap profile format](../../../../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md)
