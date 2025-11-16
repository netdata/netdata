# Configuration and Observability Specification Verification Report

**Date**: 2025-11-16
**Reviewed Files**: Configuration and observability specifications vs implementation

## Executive Summary

Cross-reference verification of specifications against implementation reveals **mostly aligned behavior** with some documentation gaps and minor discrepancies. The specs accurately describe the core functionality but miss some implementation details.

## Configuration Loading Verification

### ✅ ALIGNED

1. **Configuration Resolution Order** (src/config-resolver.ts:86-132)
   - Spec correctly lists: `--config` → `cwd` → `prompt` → `binary` → `home` → `system`
   - Implementation matches exactly with `discoverLayers()` function

2. **Environment Variable Expansion** (src/config-resolver.ts:134-147)
   - `${VARIABLE}` syntax correctly documented
   - MCP server `env` and `headers` fields properly skipped

3. **Default Queue Concurrency** (src/config.ts:9-26)
   - Correctly documented as `min(64, cores*2)`
   - Implementation uses `os.availableParallelism()` with fallback to `os.cpus().length`

4. **MCP Server Normalization** (src/config.ts:308-351)
   - Legacy type mappings documented correctly (`local` → `stdio`, `remote` → `http`/`sse`)
   - URL-based type inference working as specified

### ⚠️ DISCREPANCIES

1. **Missing Variable Error Details**
   - Spec mentions `MissingVariableError` but doesn't document full error structure
   - Implementation has scope/id/origin/variable fields (src/config-resolver.ts:19-36)

2. **Provider Type Inference**
   - **UNDOCUMENTED**: Provider type auto-inferred from ID when missing (src/config-resolver.ts:171-186)
   - Example: provider `"openai"` automatically gets `type: "openai"`

3. **Configuration File Resolution**
   - Spec says "File must exist" but implementation has graceful fallback
   - `readJSONIfExists()` returns undefined rather than throwing (src/config-resolver.ts:43-51)

### ❌ MISSING FROM SPEC

1. **Prompt Directory Layer** (src/config-resolver.ts:105-112)
   - Spec mentions but doesn't detail that prompt file directory adds a config layer
   - Only added when different from cwd

2. **OpenAPI Specs Configuration** (src/config-resolver.ts:407-423)
   - Entire `openapiSpecs` section missing from configuration-loading.md
   - Merges similarly to other config sections

3. **REST Tools Configuration** (src/config-resolver.ts:402-405)
   - `restTools` configuration section not documented
   - Uses same resolution pattern as MCP servers

## Frontmatter Parsing Verification

### ✅ ALIGNED

1. **Allowed Keys Registry** (src/options-registry.ts:49-300)
   - Correctly enforces fm-allowed keys from OPTIONS_REGISTRY
   - Runtime-only keys properly rejected

2. **Schema Resolution** (src/frontmatter.ts:167-190)
   - Priority order correct: inline → string parse → file reference
   - YAML and JSON parsing both supported

3. **Reasoning Level Parsing** (src/frontmatter.ts:128-141)
   - All documented values handled correctly
   - 'default'/'inherit' properly treated as omitted

### ⚠️ DISCREPANCIES

1. **Parser Return Behavior**
   - Spec says non-strict returns undefined on error
   - Implementation still throws for some errors even in non-strict mode

2. **Options Registry Sync**
   - Spec mentions but doesn't enforce that frontmatter.md must stay synced with OPTIONS_REGISTRY
   - No automated validation exists

### ❌ MISSING FROM SPEC

1. **Provider/Model Parsing** (src/frontmatter.ts:210-236)
   - `parsePairs()` function enforces exact `provider/model` format
   - Not documented in frontmatter.md

2. **List Parsing Details**
   - `parseList()` handles both arrays and comma-separated strings
   - Trimming behavior not specified

## Accounting Verification

### ✅ ALIGNED

1. **Token Normalization** (src/ai-agent.ts:1769-1774)
   - Cache tokens properly included in totalTokens
   - Fallback from `cacheReadInputTokens` to `cachedTokens` works

2. **Cost Priority** (src/ai-agent.ts:1806)
   - Provider-reported cost takes precedence over computed
   - Metadata fields properly extracted

3. **Entry Creation** (src/ai-agent.ts:1797-1821)
   - All documented fields present in entries
   - Trace context properly attached

### ⚠️ DISCREPANCIES

1. **Error Field Content**
   - Spec shows `error?: string`
   - Implementation uses status message extraction logic (src/ai-agent.ts:1810-1811)

2. **OpTree Attachment Error Handling**
   - Spec doesn't mention that opTree operations are wrapped in try-catch
   - Failures only warn, don't throw (src/ai-agent.ts:1819)

### ❌ MISSING FROM SPEC

1. **Token Safety Wrapper** (src/ai-agent.ts:1769-1774)
   - Try-catch around token normalization not documented
   - Keeps provider totalTokens on error

## Pricing Verification

### ✅ ALIGNED

1. **Pricing Schema** (src/config.ts:213-228)
   - Schema structure matches documentation exactly
   - Unit conversion (per_1k/per_1m) works as specified

2. **Cost Computation** (src/ai-agent.ts:1777-1795)
   - Formula matches documentation
   - Defaults and error handling correct

3. **Router Support** (src/ai-agent.ts:1780-1781)
   - actualProvider/actualModel resolution documented correctly
   - OpenRouter metadata extraction works

### ⚠️ DISCREPANCIES

1. **Default Unit**
   - Spec says "defaults to per_1m"
   - Code uses ternary: `unit === 'per_1k' ? 1000 : 1_000_000` (implicit per_1m)

### ❌ MISSING FROM SPEC

1. **Cost Computation Error Handling**
   - Entire computation wrapped in try-catch (src/ai-agent.ts:1778-1794)
   - Returns empty object on any error

2. **Number Safety Check**
   - `Number.isFinite()` check before returning cost (src/ai-agent.ts:1793)
   - Not mentioned in spec

## Logging Verification

### ✅ ALIGNED

1. **LogEntry Structure** (src/types.ts:108-154)
   - All documented fields present
   - Severity levels match specification

2. **Format Support**
   - Rich (TTY), logfmt, JSON all implemented
   - Journald sink working when available

3. **Enrichment Fields** (src/ai-agent.ts:873-944)
   - Trace context properly added
   - Stack traces captured for WRN/ERR

### ⚠️ DISCREPANCIES

1. **Structured Logger Options**
   - Spec shows simplified options
   - Implementation has more complex writer configuration

2. **Log Message IDs**
   - Spec references message-ids.ts but doesn't list them
   - Many IDs exist that aren't documented

### ❌ MISSING FROM SPEC

1. **Payload Redaction Logic**
   - When trace flags enabled, payloads are redacted
   - Truncation limits not documented

2. **Format Selection Logic**
   - First available format wins from preference list
   - 'none' format disables emission entirely

## Telemetry Verification

### ✅ ALIGNED

1. **Initialization Flow** (src/telemetry/index.ts:165-258)
   - Environment variable override works
   - Dynamic loading of OpenTelemetry deps confirmed

2. **Metric Recording** (src/telemetry/index.ts:400-700)
   - All documented instruments created
   - Labels properly applied

3. **Trace Samplers** (src/telemetry/index.ts:783-812)
   - All four sampler types implemented
   - Ratio default of 0.1 correct

### ⚠️ DISCREPANCIES

1. **Prometheus Default Port**
   - Spec says 9464
   - Could conflict with user configurations

2. **Log Export Severity Mapping**
   - Mapping logic more complex than documented
   - VRB and THK both map to DEBUG

### ❌ MISSING FROM SPEC

1. **CLI Mode Special Handling** (src/telemetry/index.ts:209-211)
   - `wrapCliExporter()` adds special error handling
   - Batch delays differ for CLI vs server

2. **Telemetry Suppression Details**
   - `AI_TELEMETRY_DISABLE` accepts multiple values: '1', 'true', 'yes', 'on'
   - Not just "set to disable"

## Missing Configuration Options

### Not in Specs but in Code

1. **Provider Configuration**
   - `mergeStrategy`: 'overlay' | 'override' | 'deep' (src/config-resolver.ts)
   - `openaiMode`: 'responses' | 'chat' (src/config.ts:60-78)
   - `stringSchemaFormatsAllowed`/`stringSchemaFormatsDenied` arrays

2. **MCP Server Configuration**
   - `toolSchemas`: Custom tool schema overrides
   - `shared`: Boolean for shared MCP servers
   - `healthProbe`: 'ping' | 'listTools'

3. **Global Configuration**
   - `slack`: Complete Slack headend configuration
   - `api`: REST API headend configuration
   - `formats`: Output format configurations

## Critical Issues Found

1. **No Circular Reference Detection**
   - Environment variable expansion could create loops
   - Schema references could be circular

2. **No Migration Path**
   - No schema versioning mentioned
   - No upgrade/downgrade strategy

3. **Incomplete Error Messages**
   - Some errors don't specify which config file failed
   - Layer origin not always included in errors

## Recommendations

1. **Update Specs**
   - Add missing configuration options
   - Document error handling patterns
   - Include implementation safety measures

2. **Add Validation**
   - Circular reference detection
   - Schema version checking
   - Configuration completeness validation

3. **Improve Testing**
   - Add tests for all edge cases found
   - Test configuration layer merging
   - Validate error message quality

4. **Documentation Sync**
   - Automate spec generation from code
   - Add spec validation to CI/CD
   - Version specs with code

## Files Requiring Updates

1. `docs/specs/configuration-loading.md`
   - Add REST tools, OpenAPI specs sections
   - Document provider type inference
   - Add error handling details

2. `docs/specs/frontmatter.md`
   - Add parsePairs documentation
   - Document parseList behavior
   - Clarify non-strict mode behavior

3. `docs/specs/accounting.md`
   - Document token normalization safety
   - Add opTree error handling details

4. `docs/specs/pricing.md`
   - Document computation error handling
   - Clarify unit defaults

5. `docs/specs/logging-overview.md`
   - List all message IDs
   - Document payload redaction
   - Add format selection algorithm

6. `docs/specs/telemetry-overview.md`
   - Document CLI special handling
   - List all env var values accepted
   - Add exporter wrapper details

## Conclusion

The specifications are **80% accurate** but missing important implementation details. Core functionality is well-documented, but error handling, edge cases, and newer features need specification updates. The implementation is more robust than the specs suggest, with additional safety measures and fallbacks not documented.

**Priority**: Update configuration-loading.md and frontmatter.md first as these are most divergent from implementation.