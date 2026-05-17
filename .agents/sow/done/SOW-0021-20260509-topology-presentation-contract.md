# SOW-0021 - Topology presentation contract

## Status

Status: completed

Sub-state: completed after inventory, schema/docs/skill updates, producer presentation updates, handoff documents, review rounds, and runtime/user validation of the repaired presentation behavior.

## Requirements

### Purpose

Restore polished, backend-controlled topology visualization while preserving the compact `netdata.topology.v1` payload model. The schema must let topology producers compose visual behavior from UI-supported tokens and profiles, without making the frontend know domain-specific concepts such as parent, child, router, client, server, process, switch, or endpoint.

### User Request

The user reported that the new compact topology schema lost important visual behavior from the prior topology payloads:

- actor fill color, border color, annotation ring color, SVG/icon choice, and popover synthesis;
- link color, line shape, curve direction, width, arrow direction, and popover synthesis;
- streaming highlight-path behavior;
- legend and color coding;
- actor bullets/ports/sockets;
- actor display-name synthesis, including a current failure where one actor label becomes a long concatenation of many MAC addresses;
- Cloud aggregator behavior for conflicting presentation/type definitions when multiple topology payloads are merged;
- Cloud aggregator matching strategies across payloads, for example socket endpoint matching, SNMP port/MAC matching, streaming machine identity matching, and future domain-specific matching.

The user explicitly split the work:

- SOW-0021: fix topology presentation.
- SOW-0022: fix table and actor-modal composition.

### Assistant Understanding

Facts:

- `netdata.topology.v1` reduced topology payload size by removing repeated row/object data and by separating graph links from evidence.
- The old topology schema contained presentation metadata for actor types, link types, port types, legends, actor click behavior, actor modal tabs, and table hints.
- The new topology schema currently has actor/link/evidence/table/overlay type registries, but no equivalent visual presentation contract.
- The UI must remain domain-agnostic. It should provide rendering tokens/enums and rendering primitives, while the backend payload chooses how actor/link/table types use them.
- Raw topology payloads and user examples may include hostnames, MAC addresses, IP addresses, interface aliases, private infrastructure details, usernames, masked passwords, and other sensitive or identifying data. Durable artifacts must only contain sanitized summaries.

Inferences:

- The current schema is strong for compact aggregation facts but weak for visual semantics.
- Hardcoding domain names and producer-specific behavior in the UI would make the schema generic only on paper.
- Reintroducing the old presentation object verbatim would preserve behavior quickly but would also preserve old schema ambiguity and bloat.
- A compact, enum/token-based presentation plane attached to type registries is the likely correct replacement.

Unknowns:

- The full set of UI rendering tokens/enums available or required in `cloud-frontend`.
- The exact conflict policy Cloud aggregation should use when payloads define the same actor/link/table type id with incompatible presentation profiles.
- The complete inventory of old schema fields and frontend consumers that must be preserved, replaced, or deliberately dropped.

### Acceptance Criteria

- Inventory all old topology presentation fields, producer emissions, and frontend consumers, including legend, colors, icons, ports/bullets, highlight behavior, popovers, actor labels, link styles, and actor modal/table references.
- Classify every old presentation capability as: preserve in v1, replace with a compact token/profile, move to SOW-0022, derive in UI from explicit backend tokens, or intentionally drop with evidence.
- Run the requested external reviewers after the inventory: Claude, Codex, GLM, MiMo, Kimi, MiniMax, and Qwen. Prompts must be shown before execution, must be read-only, must include the SOW filename, and must ask reviewers to find missing presentation semantics, unwanted side effects, security/privacy issues, and aggregator conflict risks.
- Extend `FUNCTION_TOPOLOGY_SCHEMA.json` with compact presentation/profile contracts that are enum/token based, not raw CSS/layout.
- Extend `FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`, `.agents/sow/specs/topology-function-schema.md`, and `docs/netdata-ai/skills/create-topology/SKILL.md` so future topology producers know how to define presentation profiles.
- Define Cloud aggregation merge/conflict policy for presentation profiles. Cross-payload actor reconciliation is tracked by SOW-0023.
- Create worker handoff documentation for the Cloud frontend and Cloud topology aggregator.
- Update backend producers enough to emit the new presentation contract for existing topology producers covered by this SOW.
- Validate schema, docs, skill, and backend producer output with targeted tests or equivalent checks.

## Analysis

Sources checked:

- `src/plugins.d/FUNCTION_UI_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `.agents/sow/specs/topology-function-schema.md`
- `docs/netdata-ai/skills/create-topology/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/sow/current/SOW-0020-20260505-network-connections-topology-cloud-errors.md`

Current state:

- The old schema had `topology_presentation_actor_type` with `label`, `color_slot`, `opacity`, `border`, `role`, `size_by_links`, `show_port_bullets`, `icon_svg`, summary fields, tables, and modal tabs.
- The old schema had `topology_presentation_link_type` with `label`, `color_slot`, `opacity`, `width`, and `dash`.
- The old schema had `topology_presentation_port_type`, `topology_presentation_legend`, and `actor_click_behavior` with `highlight_connections` and `highlight_path`.
- The new schema currently defines actors, links, evidence, detail tables, overlays, and aggregation metadata, but it has no presentation/profile plane.
- The topology spec currently says the new network-connections producer no longer emits superseded presentation metadata. This SOW supersedes that statement by replacing old presentation metadata with compact v1 presentation profiles.

### Inventory - 2026-05-09

Old schema and shared Go model:

- `src/plugins.d/FUNCTION_UI_SCHEMA.json:277-324` defines actor, link, and port presentation types. Required actor/link/port fields include labels and `color_slot`; optional fields include opacity, border, role, size-by-links, port bullets, icon SVG, link width, and dashed links.
- `src/plugins.d/FUNCTION_UI_SCHEMA.json:349-363` defines presentation table metadata, including `source: "data" | "links"`, `bullet_source`, and display columns.
- `src/plugins.d/FUNCTION_UI_SCHEMA.json:376-424` defines legend entries, legend sections, and `actor_click_behavior: "highlight_connections" | "highlight_path"`.
- `src/go/pkg/topology/types.go:57-136` mirrors the old presentation model in Go and states that it tells the UI how to render topology. This is useful as the old inventory source, but not the final v1 shape.
- `src/go/pkg/topology/types.go:114-136` includes `port_fields` as Go-only presentation metadata. This is not represented in `FUNCTION_UI_SCHEMA.json`, but the UI consumes it for port bullet tooltips, so it is part of the real contract.

Old producer emissions:

- Network-connections old producer emitted:
  - actor profiles for `self`, `process`, and `endpoint`;
  - `self` and `process` used `size_by_links`;
  - `process` used `show_port_bullets`;
  - process socket table used `bullet_source`;
  - socket and ownership link types had color/width/dash settings;
  - port type `topology`;
  - actor/link/port legend;
  - `actor_click_behavior: "highlight_connections"`.
  Evidence: `git show HEAD:src/collectors/network-viewer.plugin/network-viewer.c`, lines 1734-2119 in the checked pre-v1 version.
- Streaming old producer emitted:
  - actor profiles for `parent`, `child`, `vnode`, and `stale`;
  - parent used `show_port_bullets`, child/vnode/stale disabled it;
  - link profiles for `streaming`, `virtual`, and `stale`;
  - port profiles for `streaming`, `virtual`, and `stale`;
  - actor/link/port legend;
  - `actor_click_behavior: "highlight_path"`;
  - stream-path/retention/inbound/outbound table display metadata.
  Evidence: `git show HEAD:src/web/api/functions/function-topology-streaming.c`, lines 302-414 and 738-936 in the checked pre-v1 version.
- SNMP old producer still has explicit presentation code:
  - device-like actor types map labels and color slots for router/switch/firewall/access point/server/storage/load balancer/printer/phone/UPS/camera;
  - device actor profiles use border, `size_by_links`, and `show_port_bullets`;
  - segment/endpoint profiles use their own colors/roles;
  - link types include LLDP/CDP/bridge/FDB/STP/ARP/SNMP/probable;
  - port fields and port type profiles drive port bullet popovers and legend.
  Evidence: `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_types.go:7-134`.
- SNMP emits `port_fields` with labels for `type`, `role`, `status`, `mode`, and `sources`, and the Function config attaches the presentation with `WithPresentation()`.
  Evidence: `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_types.go:74-82` and `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation.go:16-50`.
- SNMP old producer also defines curated summary fields and table columns for device, segment, endpoint, ports, and links.
  Evidence: `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_schema.go:7-96`.
- The vSphere worktree also has an old-schema topology producer that must be migrated after coordination with the worker in that directory. It defines actor profiles for datacenters, clusters, hosts, VMs, datastores, networks, datastore clusters, and resource pools; link profiles for `contains`, `connects`, and `runs`; a legend; and `actor_click_behavior: "highlight_connections"`.
  Evidence from the vSphere worktree: `src/go/plugin/go.d/collector/vsphere/func_topology_presentation.go:7-83`.
- The vSphere producer attaches the old presentation to the Function method with `WithPresentation()` and emits inventory actors/links using the old topology package, so it is a real migration consumer, not only dead presentation code.
  Evidence from the vSphere worktree: `src/go/plugin/go.d/collector/vsphere/func_topology.go:33-42` and `src/go/plugin/go.d/collector/vsphere/func_topology.go:74-245`.

Current v1 schema and producers:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:220-243` has a type registry for actor/link/evidence/table/overlay/aggregation types, but no presentation/profile registry.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:304-399` defines actor and link semantics for identity, layer, direction, and aggregation, but no labels, colors, icons, line styles, bullets, legend, or highlight behavior.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:434-523` defines evidence and table type roles/columns/aggregation, but no modal or table display composition.
- `src/go/pkg/topology/v1/types.go:72-116` mirrors that same v1 gap in producer-side Go types.
- Current v1 network-connections emits a `display_name` actor column and socket evidence columns, but no presentation profile.
  Evidence: `src/collectors/network-viewer.plugin/network-viewer.c:2130-2172`.
- Current v1 streaming emits display names, machine GUIDs, link/evidence/table types, and stream-path actor tables, but no presentation profile or highlight-path contract.
  Evidence: `src/web/api/functions/function-topology-streaming.c:1043-1272`.
- Current v1 SNMP adapter emits actor metadata as compact rows and JSON table data, but no v1 presentation profile.
  Evidence: `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:386-407` and `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:451-483`.

Current cloud-frontend behavior:

- Legacy normalizer still has custom display-name synthesis from attributes, labels, and match fields; it avoids using raw match arrays as the first display choice.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/legacy/normalizeLegacyTopology.js:96-121`.
- V1 actor normalizer has a weaker `deriveLabel()` fallback. If `display_name`, name, hostname, address, or id are absent, it can fall back to actor type or generated identity; arrays are joined with commas by `safeString()`. This is consistent with the user-observed long aggregated MAC actor label class.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/v1/buildActors.js:9-18` and `src/domains/functions/topology/v1/buildActors.js:46-56`.
- V1 actor rows and nodes expose raw decoded row values directly as attributes.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/v1/buildActors.js:106-129`.
- The force graph still consumes old presentation keys for click behavior, port bullets, actor visuals, icon SVG, port colors, and legend.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/graph/forceGraph.js:601-645`, `src/domains/functions/components/graph/forceGraph.js:711-754`, and `src/domains/functions/components/graph/forceGraph.js:756-818`.
- The graph legend is entirely driven by `presentation.legend` plus actor/link/port profile maps.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/graph/graphLegend.js:41-104`.
- Color slots are already UI-owned tokens, not backend hex colors. The backend currently chooses slot names and the UI resolves them to theme colors/widths/opacities.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/colorSlots.js:9-65`.
- The current frontend color slot vocabulary is `primary`, `secondary`, `accent`, `self`, `neutral`, `muted`, `dim`, `derived`, `info`, `structural`, and `warning`. Old network-connections, streaming, and SNMP use these tokens, but the vSphere worktree uses hue names such as `blue`, `green`, `orange`, `purple`, `cyan`, `yellow`, `teal`, and `gray`, which are not present in the current frontend slot table.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/colorSlots.js:9-65`. Evidence from the vSphere worktree: `src/go/plugin/go.d/collector/vsphere/func_topology_presentation.go:10-23`.
- The canvas currently draws straight links only. It supports line color, opacity, width, and dash via presentation link types, but it has no generic curve/arrow enum yet.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/graph/forceGraphCanvas.js:63-82` and `src/domains/functions/components/graph/forceGraphCanvas.js:132-205`.
- The current frontend icon token vocabulary is closed and UI-owned: `router`, `switch`, `firewall`, `access_point`, `server`, `storage`, `load_balancer`, `printer`, `phone`, `ups`, `camera`, `process`, `agent`, `netdata-agent`, `parent`, `remote-endpoint`, `local-endpoint`, `segment`, `self`, `ip`, `cloud`, `container`, `vm`, `database`, and `service`.
  Evidence from the cloud-frontend worktree: `src/domains/functions/topology/icons.js:5-260`.
- Current v1 actor modal bypasses legacy presentation table tabs and shows a single `V1 data` tab.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/topology/actorModal/index.js:97-107`, `src/domains/functions/components/topology/actorModal/index.js:152-162`, and `src/domains/functions/components/topology/actorModal/index.js:319-364`.
- Current v1 actor modal renderer stringifies objects and overlays directly, which explains raw JSON leaking into final UI.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/topology/actorModal/V1ActorPanel.js:26-30`, `src/domains/functions/components/topology/actorModal/V1ActorPanel.js:130-168`, and `src/domains/functions/components/topology/actorModal/V1ActorPanel.js:257-263`.
- Legacy `summary_fields` are rendered by the frontend actor modal info panel, and legacy `port_fields` are rendered by the graph port bullet tooltip path. This means both are presentation-adjacent contracts even if full table/modal composition remains SOW-0022.
  Evidence from the cloud-frontend worktree: `src/domains/functions/components/topology/actorModal/index.js:277-282` and `src/domains/functions/components/graph/forceGraph.js:1015-1044`.

Current cloud-topology-service behavior:

- The service schema copy has no presentation fields in `Data`, `TypeRegistry`, `ActorType`, `LinkType`, `EvidenceType`, or `TableType`.
  Evidence from the cloud-topology-service repo: `internal/topology/schema/payload.go:29-110`.
- The aggregation core merges type registry definitions by normalized deep equality and returns a hard error for conflicting definitions.
  Evidence from the cloud-topology-service repo: `internal/topology/aggregate/aggregate.go:984-1048`.
- The aggregation spec already says type registry entries with the same id must be semantically compatible, and conflicting definitions are aggregation errors.
  Evidence from the cloud-topology-service repo: `.agents/sow/specs/cloud-topology-service-contract.md:121-123`.
- The current aggregation core merges actors by actor type plus `merge_identity`, otherwise `identity`, and links by remapped endpoints plus link type and direction policy.
  Evidence from the cloud-topology-service repo: `internal/topology/aggregate/aggregate.go:196-246`, `internal/topology/aggregate/aggregate.go:249-300`, and `.agents/sow/specs/cloud-topology-service-contract.md:124-128`.
- The service contract says evidence type match columns preserve exact relationship details, but the current model does not yet define a cross-payload matcher strategy that can replace one topology's endpoint with another topology's actor using domain-specific keys.
  Evidence from the cloud-topology-service repo: `.agents/sow/specs/cloud-topology-service-contract.md:103-116`.

Gap classification:

- Preserve in v1 as compact presentation/profile metadata:
  - actor type label;
  - actor fill color token;
  - actor border color/style token;
  - actor annotation ring token;
  - actor role/render role;
  - actor icon token;
  - actor size-by policy;
  - actor label/display-name synthesis policy;
  - link type label;
  - link color token;
  - link line shape token;
  - link curve token;
  - link width token;
  - link direction/arrow token;
  - port/bullet type label and color token;
  - legend entries/order;
  - graph selection/highlight behavior.
- Replace old fields with safer tokens/profiles:
  - old `color_slot` stays as token semantics but should become explicit enum/profile vocabulary;
  - old raw `icon_svg` should become UI-owned `icon` token unless a controlled signed/allowlisted custom icon registry is explicitly approved;
  - old boolean `dash` should become line shape enum such as `solid`, `dotted`, `dashed`;
  - old numeric `width` should become width token such as `thin`, `normal`, `thick`, or bounded scalar if the UI team confirms safe limits;
  - old `actor_click_behavior` should become a selection/highlight profile with composition rules, not a single global string.
- Move to SOW-0022:
  - summary field composition;
  - table columns, column formatters, and modal tab composition;
  - raw JSON/nested array rendering rules;
  - actor/link modal table grouping;
  - derived relationship evidence drilldowns;
  - safe display of endpoint objects, neighbors, port inventory, and overlays.
- Add because old schema did not cover enough:
  - actor `label_policy` / display synthesis, to prevent canonical identity arrays from becoming actor labels;
  - explicit note that cross-payload actor/link reconciliation is structural and tracked by SOW-0023;
  - presentation conflict policy for Cloud aggregation;
  - link curve and arrow tokens for bidirectional/directed rendering;
  - popover profile references for actor/link hover summaries;
  - annotation ring semantics for status/classification overlays.
- Add to the preserve/replace matrix because reviewers and local verification found missing old behavior:
  - `port_fields`, at least as UI label metadata for port bullet tooltip fields;
  - actor/link/port opacity semantics, preferably as closed opacity tokens rather than arbitrary floats;
  - old `topology_match` display-relevant fields as input to the new `label_policy`, not as a raw object to recreate. Cross-payload identity vocabulary moves to SOW-0023.
- Intentionally keep out of the payload:
  - coordinates, force-layout physics, viewport, pan/zoom, z-index, CSS class names, raw theme colors, raw CSS, component names, and user runtime interaction state.

Required design outputs before implementation:

1. Presentation profile schema attached to type registry entries and/or a top-level presentation registry.
2. UI token vocabulary for actor/link/port visuals that is stable and documented.
3. Label synthesis policy that separates canonical identity from human display.
4. Highlight profile schema for direct-neighbor, path, and future neighborhood behaviors.
5. Cloud aggregation conflict policy for presentation profile disagreements.
6. SOW-0022 handoff with the modal/table composition inventory above.
7. SOW-0023 handoff for cross-payload actor reconciliation.

Risks:

- If presentation stays out of the payload, the UI will need producer/domain-specific hardcoding and future topologies will not render consistently.
- If raw CSS, SVG, layout coordinates, or frontend component names enter the payload, the schema will couple backend producers to frontend implementation details.
- If Cloud aggregation accepts conflicting profiles silently, merged topologies may show inconsistent colors, icons, arrows, legends, or highlight behavior.
- Until SOW-0023 teaches Cloud cross-payload match strategies, Cloud cannot safely replace endpoints from one topology with actors from another.
- If actor labels are synthesized from canonical identity without display rules, the UI can show unusable labels such as long concatenated identity lists.
- If presentation fields allow raw untrusted HTML/SVG, the UI could get a security-sensitive rendering surface.
- If the schema is designed only around the three current in-tree producers, the vSphere topology in the companion worktree will either need a special UI path or will regress during migration.

## Pre-Implementation Gate (Historical Snapshot at Implementation Start)

Status at implementation start: ready-for-implementation (historical snapshot; final closure evidence is in the Validation and Outcome sections).

Problem / root-cause model:

- The compact schema removed the old presentation plane together with verbose compatibility data. This improved payload size but also removed backend-controlled visual semantics that the UI needs to render generic topologies without producer-specific hardcoding.
- Actor display names and labels are currently not modeled as a first-class presentation contract. Producers may emit canonical or aggregated identities that are correct for matching but unusable as human labels.
- Cloud aggregation cannot safely merge presentation profiles until the schema declares how profiles are identified, versioned, and resolved.

Evidence reviewed:

- `src/plugins.d/FUNCTION_UI_SCHEMA.json` defines the old topology presentation objects.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` lacks equivalent presentation/profile definitions.
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` currently warns against visual layout hints but does not distinguish UI-owned layout from backend-owned visual semantics.
- User-provided UI observation shows raw/aggregated identity and raw data currently leak into final topology presentation. Raw examples are intentionally not copied into this durable artifact.

Affected contracts and surfaces:

- `netdata.topology.v1` JSON Schema.
- Topology producer output for network-connections, streaming, SNMP/L2, and future vSphere topology.
- Cloud frontend topology renderer, graph legend, popovers, highlights, actor labels, and icon/color/line token mappings.
- Cloud topology aggregation service type/profile merge logic.
- Public `create-topology` skill and developer guide.
- Topology schema spec under `.agents/sow/specs/`.

Existing patterns to reuse:

- Old `FUNCTION_UI_SCHEMA.json` presentation metadata as inventory input, not as a direct replacement.
- New v1 type registry model in `FUNCTION_TOPOLOGY_SCHEMA.json`.
- Existing compact-table and type-registry helpers in `src/go/pkg/topology/v1`.
- Existing topology producer split between graph links, evidence, tables, and overlays.
- UI-token model proposed by the user: UI provides enums/primitives; backend composes profiles using those enums.

Risk and blast radius:

- Schema changes affect every producer and both Cloud consumers.
- The frontend must support old and new schemas during rollout.
- Cloud aggregation must not conflate incompatible visual/type definitions.
- Existing large-payload gains must not be lost by adding repeated per-row presentation data.
- Raw topology captures can contain sensitive customer/infrastructure details and must remain out of durable artifacts.

Sensitive data handling plan:

- Do not copy raw topology examples, raw MAC/IP addresses, interface aliases, hostnames, usernames, passwords, tokens, node IDs, claim IDs, or customer-identifying data into SOWs, specs, docs, skills, code comments, commits, or PR text.
- Use sanitized summaries and generic examples only.
- Keep raw captured payloads under `.local/` only.
- If fixtures are needed, generate sanitized fixtures with placeholder identifiers and no real infrastructure values.

Implementation plan:

1. Inventory old presentation contract, old producer emissions, current v1 schema gaps, and frontend consumption points.
2. Run the requested external read-only reviewers against the inventory and this SOW.
3. Resolve presentation schema decisions, especially profile shape, token sets, label synthesis, highlight composition, and Cloud aggregation conflict policy.
4. Extend schema/docs/spec/skill with compact presentation profiles and SOW-0023 handoff notes for cross-payload matching.
5. Create UI and Cloud aggregator worker handoff documents.
6. Update backend producers to emit the presentation profiles and validate outputs.

Validation plan:

- JSON Schema validation for fixtures using presentation profiles.
- Targeted tests for Go topology v1 helpers if helper structs/builders change.
- Function validation for at least one producer output per topology kind touched.
- Same-failure scan for old visual fields and current v1 gaps.
- Reviewer pass before schema freeze and, if material changes are made after reviewer findings, repeat review with the same scope plus fix notes.

Artifact impact plan:

- AGENTS.md: no expected update unless workflow rules change.
- Runtime project skills: likely no update except existing `project-writing-collectors` references if topology producer workflow changes.
- Specs: update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: update `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`; no public user docs expected unless Function schema docs are published.
- End-user/operator skills: update `docs/netdata-ai/skills/create-topology/SKILL.md`.
- SOW lifecycle: SOW-0020 paused; SOW-0021 current; SOW-0022 pending for table/modal composition.

Open-source reference evidence:

- `kiali/kiali @ ad210d7fd2a4b819e6ceae5f9a744847c4dcc7b2`, `frontend/src/types/Graph.ts:384-451` models graph nodes and edges with domain-specific fields such as node type, namespace, traffic, health, and source/target. This is useful evidence that mature topology UIs often carry semantic graph data, but it is not a generic presentation-token contract Netdata can copy directly.
- `apache/skywalking @ 4890024b6cc1c222838b5ebd16e10938762cd7f2`, `oap-server/server-core/src/main/java/org/apache/skywalking/oap/server/core/query/type/Node.java:27-58` and `oap-server/server-core/src/main/java/org/apache/skywalking/oap/server/core/query/type/Call.java:31-143` keep backend topology facts around nodes, calls, components, and detect points.
- `apache/skywalking-booster-ui @ 0dfb65bad317fae75353dc4ff89fae663d5e1dc5`, `src/types/topology.ts:18-74` includes UI-side fields such as positions and lower-arc hints. This supports keeping coordinates, physics, and layout state out of Netdata producer payloads while still allowing backend-owned semantic style tokens.

Open decisions:

- Resolved in `## Implications And Decisions` on 2026-05-09. Remaining implementation details are schema design details unless new evidence exposes another product decision.

## Implications And Decisions

1. User decision: split the remediation into two SOWs.
   - SOW-0021 fixes topology presentation.
   - SOW-0022 fixes table and actor-modal composition.
   - Reason: presentation profiles and table composition are related but separable contracts, and separating them allows smaller reviewable changes.

2. User decision: UI must be domain-agnostic.
   - The UI provides enums/tokens/primitives.
   - Backend payloads compose actor/link/highlight/modal behavior using those tokens.
   - The UI must not hardcode meanings such as parent, child, router, client, server, or process.

3. User decision: new compact topology payloads are all-inclusive for presentation.
   - Production `netdata.topology.v1` payloads carry presentation definitions.
   - Function `info` must not be the production transport for the new presentation contract.
   - Function `info.presentation` may exist only for legacy schema compatibility during rollout.

4. User decision: actor/link/etc. type definitions are the collision surface, not actor/link rows.
   - Payloads define actor types, link types, port types, and presentation profiles once.
   - Actor and link rows only reference those definitions.
   - Cloud aggregation conflicts are about contradictory type/profile definitions, for example two payloads defining the same `router` type differently.

5. User decision: raw SVG/custom icon payloads are not allowed in the new compact schema.
   - Use UI-owned icon tokens only.
   - Raw SVG, raw CSS, and producer-owned rendering code are excluded from `netdata.topology.v1` presentation.

6. Scope decision: keep SOW-0021 and SOW-0022 separate, but make the boundary explicit.
   - SOW-0021 owns graph presentation, safe actor labels, legend, highlight behavior, link style tokens, and port/socket bullets needed on the graph.
   - SOW-0022 owns full actor/link modal composition, custom tables, shown/hidden columns, formatters, nested JSON rendering, and curated drilldown views.
   - Rationale: merging the SOWs would increase the review surface and risk mixing graph rendering with table composition. The split is safe only if SOW-0021 provides the graph-facing hooks and SOW-0022 consumes them without redefining them.

7. Scope decision: cross-payload actor reconciliation becomes SOW-0023.
   - SOW-0021 documents the requirement but does not implement the matcher.
   - SOW-0023 will define cross-payload identity vocabularies, matching strategies, ambiguity handling, endpoint replacement rules, and Cloud aggregator behavior.

8. User decision: Cloud presentation conflicts should prefer the newest producer/schema definition.
   - Producers must expose enough version information for Cloud to compare presentation/type definitions deterministically.
   - Cloud should prefer the newer Netdata/function/schema definition when the same presentation/type id has conflicting visual definitions.
   - If version comparison cannot break the tie, Cloud may choose one deterministic fallback profile and record diagnostics.
   - Structural facts are not considered contradictions only because two producers do not correlate; they either match or remain separate valid observations.
   - Superseded by decision 9 for raw aggregation: newest-wins can be used only as a display-preference heuristic after canonicalization, never to drop facts, rows, or distinct definitions.

9. User decision refinement: type/profile definition collisions should be namespaced, then deduplicated.
   - Cloud should treat producer-local type/profile ids as local names until aggregation resolves them.
   - Cloud should canonicalize definitions and deduplicate identical definitions by content.
   - If two local ids have different definitions, Cloud should keep both by assigning distinct canonical ids instead of failing aggregation or dropping one.
   - This creates no data loss and no fatal definition conflicts.
   - Version preference can still be used later to choose a preferred display profile when multiple variants are semantically equivalent, but raw aggregation must preserve all variants.

10. User decision: link width/opacity can be data-scaled, but scaling is keyed per link type.
   - Link type definitions declare whether a visual channel is variable and the scale key it belongs to.
   - Link rows may carry one raw numeric weight for that declared key.
   - The raw number is producer-domain-specific, for example traffic, packet count, socket count, or another unit.
   - Producers do not pre-scale to pixels or opacity because each payload does not know the whole aggregated range.
   - Cloud/UI scale visual width or opacity per scale key across visible links that share that key.
   - Only one variable visual weight is allowed per link row in this SOW.

11. Link scaling examples accepted for schema design:
    - A `bandwidth` link type can declare scale key `traffic`; link rows carry raw traffic such as KiB.
    - A `connections` link type can declare scale key `sockets`; link rows carry raw socket counts.
    - These two link types can coexist because Cloud/UI scale `traffic` links against other `traffic` links and `sockets` links against other `sockets` links.

12. User clarification: link rows remain compact table rows, not per-link JSON objects.
    - Example object notation in discussion is illustrative only.
    - Production link data stays in the compact table shape: columns plus column encodings.
    - The link row carries a link type reference and, if the type declares a variable visual channel, the raw numeric value column declared by that link type.

13. User clarification and schema decision: link type and scale key are separate concepts.
    - `type` identifies the semantic link type, such as dependency, bandwidth, connections, LLDP, ownership, or streaming.
    - `scale_key` groups raw numeric weights that can be scaled together, such as `traffic` or `sockets`.
    - Multiple link types may share one `scale_key`.
    - A link type may have no variable scale key.
    - Link rows should not repeat `scale_key`; Cloud/UI resolve `row.type -> link_type.presentation.variable.scale_key`.
    - Link type values in compact tables should use dictionary/string-ref encoding so repeated type names are stored as compact indexes in row data.
    - Scale-key definitions are small type-level metadata. They may use stable ids, but clarity is preferred over numeric-only ids unless measurement shows type-level definitions matter for payload size.

14. User decision: presentation is attached inside actor, link, and port type definitions.
    - Each actor, link, and port type can carry a `presentation` object that defines backend-selected UI tokens for that type.
    - This keeps a type as one object that defines both its structural meaning and its visual profile.
    - Cloud aggregation still treats producer-local ids as local names, namespaces them, and deduplicates canonical definitions.
    - If two payloads use the same local id with different presentation, Cloud keeps both as distinct canonical ids instead of failing or dropping one.
    - Structural identity and display presentation remain conceptually separate even though they live in the same type object.

15. User decision: execution order is SOW-0021, then SOW-0023, then SOW-0022.
    - SOW-0021 will finish graph presentation, safe labels, legend, highlighting, type-level presentation, and graph-facing port/socket bullets first.
    - SOW-0023 will then solve cross-payload identity, matching, correlation, ambiguity policy, and endpoint replacement before full modal/table composition.
    - SOW-0022 will then complete actor/link modal and table composition on top of the presentation and identity foundations.
    - Reason: actor identity and naming affect topology shape, but the immediate graph regression still needs the SOW-0021 presentation contract first.

16. Identity guardrail for SOW-0021:
    - Actor identity is stable and canonical; it is never used directly as display text unless explicitly marked safe by the producer.
    - Display labels come from explicit presentation/label policy.
    - Type/profile ids are producer-local until Cloud namespaces and deduplicates them.
    - Cross-payload actor reconciliation is not implemented in SOW-0021, but SOW-0021 must not add presentation rules that block SOW-0023.

17. vSphere migration posture:
    - The vSphere topology in the separate worktree is not migrated by SOW-0021.
    - SOW-0021 keeps the vSphere-required color/icon tokens in the schema so the later migration does not need another schema round.
    - The frontend handoff explicitly requires fallback and concrete mappings for these tokens before vSphere uses them.

## Reviewer Findings - 2026-05-09

Seven requested read-only reviewer agents were run in parallel. Their raw outputs are stored under `.local/audits/topology-presentation-contract/reviews/` and are intentionally not committed.

Consolidated findings:

1. Presentation transport is unresolved.
   - Old `topology.Presentation` is Function `info` metadata.
   - Current cloud-frontend reads `response.presentation` from the Function info response.
   - The Cloud topology service aggregates topology payload `data`, not Function info metadata.
   - Risk: putting v1 presentation in the wrong place can make either the UI or the aggregator blind to it.

2. Presentation should not be mixed into structural type definitions without an explicit policy.
   - Cloud aggregation currently hard-errors on conflicting type definitions.
   - If visual fields are embedded in `actor_types` or `link_types`, a harmless color/label difference can become an aggregation failure.
   - A separate presentation registry allows structural semantics and visual semantics to have different merge rules.

3. Port bullets are graph presentation, not only table composition.
   - Old producers used `show_port_bullets`, `port_types`, `port_fields`, and `bullet_source`.
   - The frontend uses `port_fields` and `port_types` in the force graph tooltip path.
   - SOW-0021 must preserve enough port-bullet metadata for graph polish; SOW-0022 can still own full modal/table composition.

4. Actor labels need a first-class safe label policy.
   - Current v1 frontend fallback joins arrays and can expose long identity lists as labels.
   - The policy must define safe source columns, fallback order, max length, array rejection or summarization, and what the UI does when no label source is safe.

5. Raw SVG must not remain an open-ended producer surface.
   - The old schema allowed `icon_svg`.
   - Current frontend has regex-based SVG stripping before rendering legacy icons, but this is not a sufficient long-term security boundary.
   - Local search found no current topology producer emission of `icon_svg` in network-connections, streaming, SNMP, or the vSphere worktree. This means a closed UI-token icon model can be adopted without preserving active producer SVG output.

6. Token vocabularies must be explicit and versioned.
   - Current cloud-frontend supports color slots `primary`, `secondary`, `accent`, `self`, `neutral`, `muted`, `dim`, `derived`, `info`, `structural`, and `warning`.
   - The vSphere worktree uses color slot names not present in that vocabulary.
   - Current cloud-frontend supports a closed icon token map, but schema/docs do not list or version the tokens.

7. Cross-payload matching is structural, not just visual.
   - `merge_identity` is per actor type and does not define shared identity classes across producers.
   - Evidence `match_columns` preserve exact relationship details but do not declare how to replace one topology's endpoint with another topology's actor.
   - Network-connections, SNMP/L2, streaming, and vSphere need different identity keys and ambiguity policies.

8. Presentation conflict policy must be explicit.
   - Options include hard error, first-wins, priority-based merge, or separate profile IDs with deterministic fallback.
   - Silent merging is unsafe; hard errors on visual-only differences are operationally fragile.

9. Existing docs and skills currently contradict the new direction.
   - The topology spec says the producer no longer emits superseded presentation metadata.
   - The create-topology skill does not describe presentation profiles.
   - This SOW must update both so future producers do not repeat the regression.

10. Cloud service schema/validator drift was found outside pure presentation.
    - Agent-side v1 schema allows `json` columns.
    - SNMP v1 emits `json` columns for metadata tables.
    - The Cloud topology service validator currently does not allow `json` column type and scalar validation rejects objects.
    - This must be handed to the Cloud aggregator worker because otherwise valid Agent payloads can be rejected before presentation is considered.

11. SNMP actor subtype information needs preservation analysis.
    - The old presentation registry has distinct profiles for router, switch, firewall, access point, server, storage, load balancer, printer, phone, UPS, camera, generic device, endpoint, and segment.
    - Current v1 SNMP type registry exposes only device, endpoint, segment, and custom actor types.
    - The backend must preserve enough subtype or classification data for presentation profiles to reproduce the old color/icon distinctions where the old producer actually emitted those actor types.

12. Curve and arrow tokens require UI support but are still schema-relevant.
    - Current canvas draws straight lines and has no arrowhead rendering.
    - The user explicitly requires backend-controlled line shape, curve, width, and arrow direction.
    - The schema can define closed tokens now, but the UI worker must implement graceful fallback for unsupported tokens.

## Decision Gate - 2026-05-09

These decisions are blocking schema and backend implementation. They are written here before implementation so the user can answer by number and option letter.

Resolution note: this gate is resolved by `## Implications And Decisions`. Decision 14 supersedes the original recommendation in Decision Gate item 2: presentation is attached inside actor, link, and port type definitions, with Cloud namespacing and deduplication preventing visual-only conflicts from becoming data loss or aggregation failure.

### 1. Where does v1 presentation live?

A. **Recommended: `data.presentation` in the topology payload, with legacy Function `info.presentation` kept only for old-schema compatibility during rollout.**

- Pros: Cloud aggregator receives presentation; UI can render from the same production payload; avoids duplication after rollout.
- Cons: cloud-frontend must learn to read v1 presentation from `data`.
- Implications: Function info remains a compatibility path only; schema, docs, producer helpers, frontend, and cloud-topology-service all share one production presentation contract.
- Risks: requires coordinated UI and Cloud service updates before end-to-end validation.

B. Keep presentation only in Function `info`.

- Pros: matches old UI transport.
- Cons: Cloud aggregator cannot merge or validate presentation because it is not in topology `data`.
- Implications: cloud-topology-service cannot be the single topology path for all topology kinds.
- Risks: repeats the split-brain old/new contract.

C. Duplicate presentation in both Function `info` and topology `data`.

- Pros: easiest frontend transition.
- Cons: larger payloads and possible divergence between two copies.
- Implications: every producer must keep two presentation surfaces synchronized.
- Risks: stale info/data presentation mismatches create hard-to-debug UI behavior.

### 2. How should presentation attach to schema?

A. **Recommended: separate top-level `presentation` registry parallel to `types`.**

- Pros: structural type compatibility remains separate from visual profile compatibility; Cloud can merge profiles with a presentation-specific policy.
- Cons: actor/link/port types need explicit profile references or same-id conventions.
- Implications: schema can evolve visual profiles without redefining topology identity.
- Risks: frontend and service need one more registry to resolve.

B. Inline presentation fields inside `types.actor_types` and `types.link_types`.

- Pros: simpler producer shape.
- Cons: visual differences become type definition conflicts unless aggregation is weakened.
- Implications: type registry stops being purely structural.
- Risks: harmless visual changes can break Cloud aggregation.

C. Put presentation under `extensions`.

- Pros: fastest schema bypass.
- Cons: no validation, no stable contract, no reliable aggregation policy.
- Implications: future producers will invent incompatible shapes.
- Risks: recreates the current regression with another untyped escape hatch.

### 3. What is the icon/SVG policy?

A. **Recommended: closed UI-owned icon tokens only; raw SVG and raw CSS are banned from v1 presentation.**

- Pros: safest security boundary; compact; frontend can version tokens.
- Cons: producers can only use icons the UI exposes.
- Implications: add missing icon tokens to the UI token catalog as needed.
- Risks: a producer needing a custom icon waits for a UI token addition.

B. Allow raw SVG with sanitizer.

- Pros: preserves maximum producer flexibility.
- Cons: regex sanitization is not a strong security boundary; proper SVG sanitization is non-trivial.
- Implications: every topology payload becomes a UI rendering security surface.
- Risks: stored XSS or broken rendering through sanitizer bypasses.

C. Design a signed/allowlisted custom icon registry now.

- Pros: flexible and safer than raw inline SVG.
- Cons: substantially larger security, distribution, signing, revocation, and on-prem/offline design.
- Implications: delays this SOW.
- Risks: scope creep and incomplete security model.

### 4. What belongs to SOW-0021 vs SOW-0022?

A. **Recommended: SOW-0021 owns graph presentation and safe labels; SOW-0022 owns modal/table composition.**

- SOW-0021 includes actor/link/port profiles, legend, line shape, width, curve/arrow tokens, highlight profiles, `label_policy`, `show_port_bullets`, `port_types`, and `port_fields` needed for graph port tooltips.
- SOW-0022 includes modal tabs, summary sections, custom tables, table columns, formatters, nested JSON rendering, and raw-data suppression inside actor/link modals.
- Pros: fixes visible graph polish first while preserving the user's two-step split.
- Cons: some legacy fields, especially `summary_fields`, sit near the boundary and need explicit handoff notes.
- Implications: SOW-0021 must not attempt to fully solve actor modals; SOW-0022 must not redefine graph label/profile tokens.
- Risks: port tooltip and hover summary details need careful ownership to avoid gaps.

B. Move all summaries, popovers, port fields, and modal tabs to SOW-0022.

- Pros: strict table/modal ownership.
- Cons: SOW-0021 can restore colors but still leave bullets/popovers weak.
- Implications: the graph may remain visibly incomplete until SOW-0022.
- Risks: fails the "actor bullets/ports/sockets" part of the presentation complaint.

C. Move modal tabs and summary fields into SOW-0021 too.

- Pros: closer parity with the old `PresentationActorType` bundle.
- Cons: expands SOW-0021 into table/modal composition and violates the requested two-step split.
- Implications: SOW-0022 becomes smaller but SOW-0021 becomes much larger.
- Risks: delays the graph presentation repair.

### 5. Is cross-payload actor reconciliation in SOW-0021?

A. **Recommended: SOW-0021 documents the requirement and creates/updates a separate SOW for cross-payload matching; it does not implement the matching algorithm.**

- Pros: keeps SOW-0021 focused on presentation; acknowledges the issue honestly; avoids mixing visual schema work with structural graph reconciliation.
- Cons: aggregated multi-producer topologies may still show duplicates until the matching SOW is implemented.
- Implications: presentation schema can include only non-invasive identity labels needed by UI; matching strategy, ambiguity handling, and endpoint replacement live in the follow-up.
- Risks: future schema changes may be needed if the matching SOW requires additional producer declarations.

B. Add producer-declared match strategies to v1 now, but leave Cloud implementation to a later service change.

- Pros: future-proofs the payload.
- Cons: unimplemented schema fields can drift or be misused.
- Implications: producers must emit strategy metadata before the aggregator consumes it.
- Risks: false confidence that matching works because the payload has declarations.

C. Include full cross-payload matching schema and Cloud aggregator algorithm in SOW-0021.

- Pros: solves the structural duplicate-actor problem now.
- Cons: significantly expands scope beyond presentation.
- Implications: SOW-0021 must define typed identity vocabularies, normalization, ambiguity policy, match confidence, and tests across all topology kinds.
- Risks: delays presentation repair and increases review surface.

### 6. What is the presentation conflict policy for Cloud aggregation?

A. **Recommended: structural conflicts stay hard errors; presentation conflicts use deterministic profile merge with diagnostics, not topology failure.**

- Pros: preserves correctness for topology identity while avoiding visual-only aggregation outages.
- Cons: a merged view may pick one visual profile when producers disagree.
- Implications: Cloud service must record conflict counts/details and choose profiles by deterministic priority, such as explicit profile priority then producer/source ordering.
- Risks: users may see a generic or first-selected style when producers disagree.

B. Hard-error on any presentation conflict.

- Pros: simplest and maximally strict.
- Cons: visual-only differences can prevent topology rendering.
- Implications: every producer version must agree exactly on profile definitions before Cloud aggregation works.
- Risks: fragile during rolling upgrades.

C. Last-writer-wins silently.

- Pros: easy implementation.
- Cons: nondeterministic unless merge order is guaranteed; hides real producer disagreements.
- Implications: UI may change colors/icons depending on aggregation order.
- Risks: confusing and hard to debug.

### 7. Should link width/opacity be tokens or bounded numbers?

A. **Recommended: closed tokens for width and opacity in v1.**

- Pros: compact, theme-owned, predictable, easy to validate.
- Cons: less granular than old numeric values.
- Implications: map old width/opacity to `thin`/`normal`/`emphasis` and `normal`/`muted`/`faded`.
- Risks: some old visual nuance may be approximated.

B. Bounded numeric values.

- Pros: closer to old schema and existing canvas math.
- Cons: producers can still tune UI details too closely.
- Implications: schema must enforce min/max and frontend must clamp.
- Risks: style drift between producers.

C. Keep old raw numeric semantics.

- Pros: simplest migration from old schema.
- Cons: keeps old ambiguity.
- Implications: backend retains too much control over frontend look.
- Risks: inconsistent and unreviewed visual scaling.

## Plan

1. Inventory old and new contracts.
2. Produce a gap matrix with preserve/replace/drop/defer classification.
3. Run requested external reviewers.
4. Extend schema and docs.
5. Write UI and Cloud aggregator handoff docs.
6. Implement backend producer support.
7. Validate and update SOW.
8. Hand execution to SOW-0023 before starting SOW-0022.

## Execution Log

### 2026-05-09

- Opened SOW-0021 from user direction after discovering the compact schema dropped required visual semantics.
- Paused SOW-0020 so presentation repair can be handled as an explicit, reviewable step.
- Created SOW-0022 as the follow-up for actor modal/table composition.
- Ran all seven requested read-only external reviewers and consolidated their findings into `## Reviewer Findings - 2026-05-09`.
- Added `## Decision Gate - 2026-05-09` because the reviewers found blocking design choices that must be answered before schema/backend implementation.
- Recorded the final execution order decision: finish SOW-0021, then run SOW-0023, then run SOW-0022.
- Recorded the SOW-0021 identity guardrail so presentation labels do not misuse canonical identity and do not block the cross-payload matcher.
- Implemented compact presentation in `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`, `src/go/pkg/topology/v1`, network-connections, streaming, and SNMP v1 producers.
- Created worker handoff docs:
  - `<cloud-frontend-repo>/TODO-topology-presentation-contract.md`
  - `<cloud-topology-service-repo>/REQUIREMENTS-topology-presentation.md`
- Created the backend-worker SOW and expanded the frontend-worker TODO so the
  other workers can port the SOW-0021 changes into their codebases:
  - `<cloud-topology-service-repo>/.agents/sow/pending/SOW-0005-20260509-port-topology-presentation-contract.md`
  - `<cloud-topology-service-repo>/SOW-status.md`
  - `<cloud-frontend-repo>/TODO-topology-presentation-contract.md`
- Answered the first Cloud frontend worker question batch by appending
  `## Answers - 2026-05-10` to
  `<cloud-frontend-repo>/TODO-topology-presentation-contract.md`.
  The answers explicitly state that the frontend remains topology-schema
  agnostic and must use representative v1 feature fixtures, not
  producer-specific UI branches.
- Created `.agents/sow/pending/SOW-0024-20260510-vsphere-topology-v1-migration.md`
  to track the later vSphere migration to `netdata.topology.v1` after
  SOW-0021, SOW-0023, and SOW-0022 are finished.
- Ran the second requested-style read-only reviewer pass after material implementation changes. Raw outputs are under `.local/audits/topology-presentation-contract/reviews-round2/`.
- Gemini returned no review output in round 2; the output file is zero bytes. The other read-only reviewers completed and their findings were consolidated locally.
- Fixed the real second-round findings: semantic presentation validation, `highlight_path` and metric-size schema conditionals, width/opacity token constraints for link variables, explicit port-bullet sources, token documentation, handoff merge rules, SNMP custom legend entry, and the network-connections ownership metrics leak.
- Ran a third read-only reviewer pass after the second-round fixes, using the same scope plus fix notes. Raw outputs are under `.local/audits/topology-presentation-contract/reviews-round3/`.
- Kimi timed out in the third pass after 30 minutes and produced only progress output, not a final review.
- Fixed the concrete third-round findings:
  - streaming port-bullet labels now use `port_name`, not raw `actor_ref` row indexes;
  - network-connections only advertises socket-evidence port bullets in detailed mode, where socket evidence is present;
  - `label_policy` rejects non-display columns such as `array`, `json`, and raw refs;
  - hover fields validate against actor/link table columns;
  - highlight-path actor/order columns are type-checked;
  - actor-table port sources must reference declared table types or runtime actor tables;
  - SNMP always declares the `actor_ports` table type for its port-bullet source and adds `unknown` to the port legend;
  - Go `BorderPresentation.Enabled` is now optional so schema defaults are not contradicted by zero-value Go structs.
  - topology/v1 tests now verify Go token arrays stay in sync with the JSON Schema token enums.
- Added the public `create-topology` how-to for graph presentation and linked it from the live how-to catalog:
  - `docs/netdata-ai/skills/create-topology/how-tos/add-graph-presentation.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`
- Investigated runtime SNMP/L2 visual regression reported after frontend
  integration: inferred/probable links were rendered like LLDP/CDP links.
  Root cause: the SNMP v1 producer collapsed all graph rows into one
  `l2_observation` link type, so the frontend had no schema-level signal to
  apply the old `probable` presentation.
- Fixed SNMP v1 link typing:
  - graph rows now preserve semantic link types for `lldp`, `cdp`, `bridge`,
    `fdb`, `stp`, `arp`, `snmp`, `probable`, and fallback `l2_observation`;
  - each link type has its own v1 presentation tokens;
  - evidence sections are split by matching evidence type so
    `evidence_types.<id>.link_type` stays coherent with the graph link type.
- Added the public `create-topology` how-to for preserving semantic link types:
  - `docs/netdata-ai/skills/create-topology/how-tos/preserve-semantic-link-types.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`
- Investigated runtime streaming highlight-path regression reported after the
  frontend integration: clicking a streaming actor highlighted only direct
  graph siblings instead of that actor's ordered streaming path.
  Root cause: the v1 streaming payload configured `path_actor_column: "actor"`
  while the `actor` column is the clicked/owner actor, not the path member; the
  frontend v1 adapter also did not materialize v1 path tables into the legacy
  `streamingPath` node field consumed by the graph click handler.
- Extended the `highlight_path` contract with optional `path_owner_column`.
  `path_actor_column` now means path member; `path_owner_column` means the
  clicked actor that owns the path row. Existing global path-table payloads
  remain valid when `path_owner_column` is omitted.
- Fixed streaming v1 path rows:
  - `stream_path` now carries both owner `actor` and member `path_actor`;
  - streaming presentation now points at `path_owner_column: "actor"` and
    `path_actor_column: "path_actor"`;
  - the v1 stream-path table mirrors the old highlight-path helper by appending
    the local agent to stored paths when storage does not already include it.
- Fixed the frontend v1 adapter in the Cloud frontend worktree:
  - validates optional `path_owner_column`;
  - resolves per-owner highlight paths from compact actor tables;
  - materializes `streamingPath` arrays onto graph nodes before `ForceGraph`
    handles click selection.
- Updated the Cloud frontend and Cloud topology service handoff documents so
  worker agents port `path_owner_column` semantics together with the backend
  schema change.
- Added the public `create-topology` how-to for per-actor highlight paths:
  - `docs/netdata-ai/skills/create-topology/how-tos/define-per-actor-highlight-paths.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`

## Validation

Acceptance criteria evidence:

- Old graph presentation semantics were inventoried from old schema, old producer emissions, and frontend consumers in `## Inventory - 2026-05-09`.
- `netdata.topology.v1` now has:
  - type-level actor/link/port presentation;
  - graph-level `data.presentation`;
  - safe `label_policy`;
  - closed icon/color/opacity/width/line/curve/arrow tokens;
  - explicit port-bullet sources;
  - legend, highlight-path, scale-key, hover, annotation, and variable link scaling contract.
- Backend producers covered by this SOW emit v1 presentation:
  - `src/collectors/network-viewer.plugin/network-viewer.c`
  - `src/web/api/functions/function-topology-streaming.c`
  - `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`
- UI and Cloud aggregator worker handoff docs exist at the absolute paths recorded in the execution log.
- Backend service handoff is now a real pending SOW in the service repository:
  `<cloud-topology-service-repo>/.agents/sow/pending/SOW-0005-20260509-port-topology-presentation-contract.md`.
- Frontend handoff is now an expanded implementation TODO in the frontend
  repository: `<cloud-frontend-repo>/TODO-topology-presentation-contract.md`.

Tests or equivalent validation:

- `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- `go test ./pkg/topology/v1 ./plugin/go.d/collector/snmp_topology ./tools/functions-validation/validate` passed from `src/go`.
- C syntax validation passed using the exact compile commands from `build/compile_commands.json` with `-fsyntax-only` for:
  - `src/collectors/network-viewer.plugin/network-viewer.c`
  - `src/web/api/functions/function-topology-streaming.c`
- After third-round fixes, the same Go test command and both C `-fsyntax-only` checks passed again.
- After the SNMP/L2 inferred-link fix,
  `go test ./plugin/go.d/collector/snmp_topology` passed from `src/go`.
- After the streaming highlight-path fix:
  - `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
  - `go test ./pkg/topology/v1 ./plugin/go.d/collector/snmp_topology ./tools/functions-validation/validate` passed from `src/go`.
  - C syntax validation passed for `src/web/api/functions/function-topology-streaming.c` using the exact compile command from `build/compile_commands.json` with `-fsyntax-only`.
  - In `<cloud-frontend-repo>`, `yarn test src/domains/functions/topology/v1/normalizeTopologyV1.test.js src/domains/functions/topology/v1/buildRenderPresentation.test.js --runInBand` passed with 83 tests.
- The Go test suite includes a schema-token parity test so future token additions must update the schema and Go validator together.
- The SNMP Go tests now decode `data.links.type` and verify LLDP rows remain
  `lldp`, probable/inferred rows become `probable`, presentation tokens differ,
  evidence type link references match the graph link type, and legend entries
  include both categories.
- Full CMake/Ninja build could not run because the local `build/` directory and Ninja files are owned by `root`; `cmake --build build --target network-viewer.plugin netdata -j 8` fails with `.ninja_lock` permission denied.
- Backend service SOW audit passed after creating
  `cloud-topology-service/.agents/sow/pending/SOW-0005-20260509-port-topology-presentation-contract.md`.

Real-use evidence:

- The user confirmed the previous SNMP/L2 inferred-link visual fix works.
- The streaming highlight-path repair is locally validated by backend schema,
  Go, C syntax, and frontend adapter tests. The user later confirmed the
  installed topology view works with the patched producer/UI path.
- Direct Cloud aggregation runtime validation remains owned by the Cloud
  topology service worker handoff, not by this netdata repository commit.
- The producer code was validated by Go tests, JSON Schema tests, function-validation fixtures, and C syntax checks. Full Cloud aggregation parity remains dependent on the Cloud topology service handoff.

Reviewer findings:

- First review round completed. Raw outputs are under `.local/audits/topology-presentation-contract/reviews/`; consolidated findings are recorded in `## Reviewer Findings - 2026-05-09`.
- Second review round completed for Codex, Claude, Qwen, GLM, MiniMax, Kimi, and MiMo. Raw outputs are under `.local/audits/topology-presentation-contract/reviews-round2/`.
- Second-round Gemini produced no output; `.local/audits/topology-presentation-contract/reviews-round2/gemini.txt` is zero bytes.
- Real findings handled in code/docs/handoffs:
  - semantic presentation cross-reference validation in `src/go/pkg/topology/v1/validate.go`;
  - schema conditional requirements for `highlight_path` and metric actor sizing;
  - link variable `min`/`max` constrained to width/opacity tokens;
  - explicit `ports.sources[]` instead of implicit/misleading port type columns;
  - token vocabulary and fallback guidance;
  - Cloud merge rules for `label_policy`, scale keys, port sources, and column-name preservation;
  - sensitive identifier handling note for streaming detail tables;
  - SNMP `custom` actor added to legend;
  - network-connections ownership link no longer declares socket-only metrics.
- Third review round completed for Codex, Claude, Qwen, GLM, MiniMax, and MiMo. Kimi timed out after 30 minutes without a final review. Raw outputs are under `.local/audits/topology-presentation-contract/reviews-round3/`.
- Third-round concrete findings handled in code/docs:
  - streaming port-bullet `name_column` now references a scalar display column;
  - network-connections no longer advertises evidence-derived port bullets when evidence is omitted in aggregated mode;
  - actor-table port sources no longer silently pass missing table declarations;
  - label policy and hover fields now reject non-display columns;
  - highlight-path path columns now require `actor_ref` and numeric order types;
  - frontend/cloud handoffs now clarify scalar bullet labels, fallback defaults, and evidence id rewriting.
- Third-round findings that remain outside SOW-0021 implementation are tracked by worker handoffs or follow-up SOWs:
  - Cloud topology service must accept Agent-valid `json` columns before end-to-end Cloud aggregation is ready;
  - frontend worker must implement token fallbacks and port-source rendering tests;
  - full CMake build and runtime Function samples require a writable build tree / installed Agent and the worker integrations.

Same-failure scan:

- `rg` scan for legacy-only presentation fields over the new v1 producers/docs found no remaining production use of `icon_svg`, `actor_click_behavior`, `summary_fields`, `bullet_source`, or `topology_match`.
- Remaining `show_port_bullets` matches are local variable names in C emitters and the migration note that maps old `show_port_bullets` to v1 `ports.show_bullets`.
- `rg` scan for `path_actor_column` still pointing at owner `actor` found no
  remaining production payload definition after the streaming fix; remaining
  matches are the SOW root-cause note, schema/spec prose, validator errors, and
  the intended `path_actor` emission.

Sensitive data gate:

- Raw user-provided examples are not copied into this SOW. This SOW uses sanitized summaries only.
- No raw topology captures were committed. Raw reviewer outputs stay under `.local/`.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow rules did not change.
- Runtime project skills: no generic `.agents/skills/project-*` update was needed; the relevant topology producer workflow is the public `create-topology` skill, updated below.
- Specs: `.agents/sow/specs/topology-function-schema.md` was updated with the presentation contract, token vocabulary, port-bullet sources, and aggregation conflict policy.
- End-user/operator docs: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` and `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` were updated for the new v1 presentation model.
- End-user/operator skills: `docs/netdata-ai/skills/create-topology/SKILL.md` was updated; `docs/netdata-ai/skills/create-topology/how-tos/add-graph-presentation.md` and `docs/netdata-ai/skills/create-topology/how-tos/preserve-semantic-link-types.md` were added and linked from `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`.
- End-user/operator skills: `docs/netdata-ai/skills/create-topology/how-tos/define-per-actor-highlight-paths.md` was added and linked from `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`.
- SOW audit hygiene: `.agents/skills/mirror-netdata-repos/SKILL.md` had an existing SSH clone example that matched the audit email-address heuristic. It was reworded without changing behavior so the sensitive-data audit passes cleanly.
- SOW lifecycle: SOW-0021 is marked `completed` and is moved to `.agents/sow/done/` with the netdata implementation commit. SOW-0020 is closed in the same netdata commit because this work built on its compact topology schema foundation. SOW-0023 and SOW-0022 remain pending follow-ups.

Specs update:

- Updated `.agents/sow/specs/topology-function-schema.md`.

Project skills update:

- No generic `.agents/skills/project-*` update was needed.
- Updated the public `create-topology` skill because it is the topology producer workflow reference:
  - `docs/netdata-ai/skills/create-topology/SKILL.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/add-graph-presentation.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/preserve-semantic-link-types.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/define-per-actor-highlight-paths.md`
  - `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`

End-user/operator docs update:

- Updated `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- Updated `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`.

End-user/operator skills update:

- Updated `docs/netdata-ai/skills/create-topology/SKILL.md`.
- Added `docs/netdata-ai/skills/create-topology/how-tos/add-graph-presentation.md`.
- Added `docs/netdata-ai/skills/create-topology/how-tos/preserve-semantic-link-types.md`.
- Added `docs/netdata-ai/skills/create-topology/how-tos/define-per-actor-highlight-paths.md`.
- Updated `docs/netdata-ai/skills/create-topology/how-tos/INDEX.md`.

Lessons:

- Closed token schemas are not enough by themselves. Semantic validation must also check that labels, legends, path references, scale keys, and port-bullet sources point to real tables, columns, and type definitions.
- Port bullets need explicit data-source declarations. A boolean `show_bullets` restores only visibility, not the data contract the UI needs.
- Dynamic actor detail tables, especially SNMP tables, require careful validation: required source columns can be checked, but optional enrichment columns must be allowed to be absent when the table shape varies by device.
- If a type-level presentation references optional runtime data, the type registry still needs a stable declaration for that source. Otherwise validation cannot distinguish an intentionally empty table from a typo.
- Graph link type is the UI's presentation handle. If a producer collapses
  visually distinct facts into a generic link type, the frontend cannot remain
  topology-agnostic and still recover the old visual meaning.
- Highlight paths that are actor-specific need two actor references: one for
  the owner/clicked actor and one for the path member. Reusing one column for
  both preserves table shape but loses the selection semantics.

Follow-up mapping:

- SOW-0022 tracks actor modal/table composition.
- SOW-0023 tracks cross-payload actor reconciliation and must run before SOW-0022.

## Outcome

Backend/schema/docs/handoff work for SOW-0021 is implemented and locally validated in the netdata repository. The schema now carries backend-selected presentation profiles for actor, link, and port types; producer output preserves semantic link types; streaming can define per-actor highlight paths; and the Cloud frontend plus Cloud topology service handoffs are documented for their owning repositories.

## Lessons Extracted

See `## Validation` lessons above.

## Followup

- SOW-0022 handles modal/table composition.
- SOW-0023 handles cross-payload actor reconciliation and is ordered before SOW-0022.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
