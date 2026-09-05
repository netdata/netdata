# Operator Surface: Config Options, Forms, And Metadata

The operator surface is a public contract: easy to add, hard to remove. This reference owns the decision and the form;
`src/go/plugin/go.d/docs/how-to-write-a-collector.md` (Config) owns the list of what stays a constant, and
`.agents/skills/integrations-lifecycle/consistency.md` owns which artifacts change together. Rules use the format
When / Do / Don't / Evidence / Boundary.

## 1. Config Decision Record

**When:** every proposed public option, including ones inherited from a shared config type. **Do:** answer two
independent questions per option and write the row before the Go field exists. First: does it represent an operator
decision, or does it export an implementation choice? Second: if it is a decision, are its name, unit, default,
placement, validation, and explanation usable by a human? **Don't:** keep an option because "it might be useful to
tune", convert an unjustified knob from milliseconds to `15m` and call it fixed (that answers only the second
question), or delete a justified objective and a justified hard timeout merely because the list has four entries.
**Evidence:** the filled row. **Boundary:** connection identity, endpoint, credentials, the target request timeout,
`update_every`, `vnode`, and selectors that scope cardinality are operator decisions by default.

| Option | Operator decision it enables | Keep? | Name / type / unit | Default | Placement (tab or mode object) | Validation |
|---|---|---|---|---|---|---|

Rules that follow from the record:

- Durations, sizes, and their types follow the how-to guide's Config section (`confopt` types, human units, no custom
  parsing); the decision record only names the unit.
- Do not inherit a broad shared config type wholesale. Every field it brings (transport, TLS, HTTP/2, retry policy,
  proxy) gets its own row or is excluded; an option accepted in a mode where it has no effect is a defect.
- What is configuration and what stays a constant is listed once, in the how-to guide's Config section; a constant
  becomes an option only when a row names the operator decision.
- Static-only credentials are a product limitation to decide, not a correctness defect; record the decision.

## 2. Option Lifecycle And Compatibility

**When:** adding, renaming, changing the default of, or removing an option on a shipped collector. **Do:** add with a
decision record; rename by keeping the old key working and documenting the deprecation in metadata; treat a default
change as a behavioral change that needs a SOW and a note in the docs; remove only under an explicitly approved breaking
decision, with `Config`, `config_schema.json`, stock `.conf`, `metadata.yaml`, and tests in one PR; generated
integration pages follow the delivery route in `consistency.md` (validated locally, committed by the post-merge
generated-artifact PR). **Don't:** let a new collector's freedom to replace its own contract before release leak into
work on a shipped collector; the V1-to-V2 migration guide's compatibility rules apply to shipped contracts.
**Evidence:** the consistency checklist run against the PR. **Boundary:** a collector that has not yet shipped in a
release may change its contract freely with the user's approval.

## 3. The DynCfg Form As A User Task

**When:** any collector with modes, providers, authentication variants, or more than a handful of options. **Do:**
start from a minimal usable YAML example and a realistic DynCfg workflow, not from the exported fields of an SDK or
options type. Decide "should this be public" before "which tab holds it". Keep a mode selector and its complete
conditional configuration object together in one tab; keep common collection options (`update_every`, `vnode`,
`timeout`) separate. Exercise the actual frontend for mode and authentication switching and verify that switching
leaves no hidden contradictory fields or secrets in the submission. Give secrets `"ui:widget": "password"` (the
`sensitive` flag is read by nothing) and confirm the form masks them. **Don't:** call a schema that validates a usable
form; nest a second discriminator inside a mode object without inspecting how the frontend prunes inactive branches
(the examined version pruned only at the root); pretend JSON Schema enforcement runs on every job when runtime
`validate()` is the authority. **Evidence:** the exercised form flows, recorded in the SOW with the frontend version.
**Boundary:** frontend behavior is version-specific; re-inspect rather than treating a past limitation as timeless.

## 4. Consumer Traces And Defaults

**When:** any field whose meaning depends on absence, `false`, zero, `null`, inheritance, or the selected mode.

**Do:**

- Trace both consumer paths before choosing field types: constructor → raw decode → `Init`, and constructor → raw
  decode → `Configuration()` → serialization and supported reload. `Configuration()` can run before `Init()`
  (`src/go/plugin/agent/jobmgr/joboutput/config_factory.go`), so do not call `Init()` first and claim to have proved
  pre-Init retrieval.
- Keep ordinary scalar defaults in `New()`; `New()` MUST return a usable collector, because callers and tests construct
  one without `Init()`.
- A config normalizer (`applyDefaults`, run from `Init`) handles only what the constructor cannot see: an explicit
  sentinel the operator typed (a `0`), or a nil pointer on a config not built through `New()`.
- For mutually exclusive branches keep the constructor mode-neutral and materialize the selected branch through that
  one owned defaulting step at the boundaries that need it; preserve explicit values; never let effective-config
  retrieval mutate the raw instance.

**Don't:**

- Preallocate a branch in the constructor and unmarshal another mode into it.
- Add a third read-time fallback (`if v <= 0 { v = default }`) that can never fire once constructor and normalizer ran.
- Treat `omitempty` as harmless for meaningful `false` or `0`.

**Evidence:** canonical wire-form fixtures `testdata/config.json` / `config.yaml` driven by
`collecttest.TestConfigurationSerialize`, carrying meaningful `false` and zero values; and, as the one explicit
exception to "no collector-local decode test", a targeted raw-input case for a boundary the struct round-trip cannot
exercise: an explicit `null` key, which a typed nil pointer with `omitempty` never serializes (the case MUST include
the key and check both schema and runtime acceptance).

**Boundary:** a nil check on a pointer is memory safety, not defaulting, and stays. Before "simplifying" defaults,
check what the constructor guarantees its callers: apply the change and count the test failures before recommending
it; production reachability alone understates the blast radius.

## 5. Schema Form Rules

`config_schema.json` is a form contract, not a validation layer; nothing in the agent validates a job against it, and
runtime enforcement is the collector's own `validate()`. Write it for the operator filling the form. How to write it
(text channels, tabs, widgets, conditional sections, secrets, standard option wording, the repo-wide rule tests) is
owned by `config-schema.md`; the renderer contract is `src/plugins.d/DYNCFG.md`, "JSON Schema for Configuration UI".
The design-time rules that stay here:

- A schema rule stricter than the runtime blocks legitimate configs in the UI; a looser one offers configs the
  collector rejects on `add`. A deliberate asymmetry is fine where only one layer can act: `minimum: 1` on an option
  whose code treats `0` as "use the default", because the form should not offer `0` while a file may legitimately
  contain it.
- An option whose omission drives runtime precedence ("unset means inherit the rule default") MUST NOT carry a schema
  `default`: the form materializes every default into the submitted job, so the omission can never happen from the UI.
- The form and the doc are two views of one vocabulary: tab titles equal `metadata.yaml` groups and option
  descriptions are identical in both (`collecttest.AssertConfigSchemaMatchesMetadata`; a tabbed collector whose
  options or descriptions you change MUST opt in).

Tests about configuration:

- Do NOT write tests that restate the schema file (its `required` list, defaults, `ui:help`, placeholders); they pin
  mistakes as correct. Only two kinds carry weight: a **rule** (a forbidden pattern that fails on a wrong schema, such
  as the repo-wide `minItems` check, or a per-collector check that a field whose omission drives runtime precedence
  carries no `default`: `cloudwatch/config_test.go`, `TestConfigSchema_FormContract`) and a **drift check** (an expected
  value taken from an independent source, such as a Go constant the UI bound must equal; copying the value out of the
  schema turns it back into a restatement).
- Test `validate()` by mutating a valid `Config` value per case. Do NOT hand-build `map[string]any` payloads or drive
  cases from YAML source text; both test the decoder, not the collector.
- Where a field's shape encodes a contract, test the accessor that reads it (`cloudwatch/config_test.go`,
  `TestRuleConfig_effectiveResourceTagFilters`). Check whether the rule already has coverage nearer its owner: a helper
  package validates its own types, and compile-stage reference rules belong with the compile tests, not the config
  tests.

## 6. Metadata And Help In Operator Voice

**When:** writing `metadata.yaml` fields, `ui:help`, and stock `.conf` comments for a collector designed with this
skill. **Do:** write what the design decided (modes, options, and objectives versus timeouts from sections 1 to 5;
what the collector writes, deletes, persists, and requires as preconditions from
`mutating-collectors.md`; the permissions the code actually requests) in operator voice, following
`.agents/skills/project-collector-metadata/` for every `metadata.yaml` field and `config-schema.md` section 2 for the
form's text channels. **Don't:** ship the engine's vocabulary as operator documentation. **Evidence:** the generated
integration page read end to end. **Boundary:** the content rules live in the collector metadata skill; artifact
mechanics and the delivery route in `.agents/skills/integrations-lifecycle/`; do not duplicate either here.
