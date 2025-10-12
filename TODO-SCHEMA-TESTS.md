# Schema Validation Test Plan

We need deterministic coverage for every code path that depends on agent/sub-agent input or output schemas.

## Required Scenarios

1. **Agent default schema (text)**  
   - Fixture without `input` or `expectedOutput` frontmatter.  
   - Assert `loadAgent(...).input` equals the shared fallback and `expectedOutput` is `undefined`.

2. **Agent explicit JSON schema + output**  
   - Fixture with `input.format: json` and custom JSON schema, plus `expectedOutput`.  
   - Verify both schemas are cloned (mutations do not leak) and exposed on the loaded agent.

3. **Sub-agent default schema (text)**  
   - Child agent using fallback schema.  
   - Ensure `SubAgentRegistry.getTools()` returns prompt+reason only (`format` fixed to `sub-agent` via defaults) with `additionalProperties: false`.  
   - `execute` should accept a payload containing both `prompt` and `reason`, reject missing `prompt`, and reject missing/blank `reason`.

4. **Sub-agent explicit JSON schema**  
   - Child agent declares its own JSON schema.  
   - Verify `getTools()` clones the schema and injects a required `reason` property while leaving all other user-defined structure intact.  
   - `execute` should enforce the injected `reason` (missing -> failure) and respect the rest of the user schema (valid payload succeeds, invalid payload fails).

5. **Sub-agent explicit JSON output schema**  
   - Child agent defines `expectedOutput`.  
   - Confirm cloned schema appears on `loaded.subAgents[i].loaded.expectedOutput`, remains isolated from caller mutations, and that runtime enforces JSON serialization.

## Success Criteria

- Each scenario runs inside the phase1 harness or equivalent deterministic test, asserting both loader metadata and runtime validation behaviour.  
- Combined coverage hits every branch that reads `input.schema`, `expectedOutput.schema`, and sub-agent Ajv execution paths.  
- No schema-dependent code remains untested; future regressions must fail one of these scenarios.
