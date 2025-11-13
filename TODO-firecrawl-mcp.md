# TL;DR
- Investigate firecrawl MCP library to confirm whether it injects unsupported JSON Schema keywords (e.g., `propertyNames`) into tool schemas and whether there is a configuration flag to disable or alter this behavior.

# Analysis
- Observed behavior: `./ai-agent ... web-fetch` failed repeatedly because the tool schema for `firecrawl-fetch__firecrawl_extract` contains the `propertyNames` keyword that vLLM's tool grammar does not implement.
- Current repository (`ai-agent.git`) does not appear to vendor firecrawl MCP; we likely consume it via MCP tooling, so we need to inspect the upstream firecrawl MCP implementation.
- The code responsible for schema emission is unknown until we download and review the firecrawl MCP source (likely from npm or GitHub). No local copy exists yet.

# Decisions Needed
- Confirm exact upstream package/repo (npm name, version) that we should inspect—assume `firecrawl-mcp` unless instructed otherwise.
- Determine whether we are allowed to modify upstream code or only configure existing options.

# Plan
1. Download or clone the `firecrawl-mcp` source into `/tmp/firecrawl-mcp` to allow inspection without polluting the main repo. **(done)**
2. Review package metadata (README, package.json) to locate configuration or schema-generation modules. **(done)**
3. Trace how tool schemas are defined, focusing on fields that add `propertyNames`; note whether flags exist to disable strict JSON Schema features. **(done)**
4. Document findings (file paths, code snippets) and propose mitigation (config option vs. patch requirement). **(done)**
5. Build tooling to quantify the blast radius: load `neda/.ai-agent.json`, enumerate each MCP server’s tool schemas (via `--list-tools` or direct loading), and validate every schema against a selectable draft (e.g., Draft‑04). Provide a report of unsupported keywords so we know all affected tools before implementing sanitization.

# Implied Decisions
- Need to decide whether sanitization should happen upstream (firecrawl MCP) or downstream (ai-agent). This task focuses on upstream options; downstream mitigation is out of scope unless no upstream option exists.

# Testing Requirements
- None executable yet; once a solution is identified, we would re-run the failing `./ai-agent ...` command to validate.

# Documentation Updates Required
- Pending outcome. If a configuration knob exists, we must document it in our project docs (e.g., README or AI-AGENT-GUIDE) after implementation.
