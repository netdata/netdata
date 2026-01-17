# AI Agent Skill Document Merge

## TL;DR
Merge three skill files (`ai-agent-configuration.md`, `ai-agent-configuration-user.md`, `ai-agent-user-guide.md`) into a single comprehensive `ai-agent-guide.md` skill document. Then review all wiki pages to enrich the skill with any missing content.

## Objective
Create a single, dense, comprehensive skill document that AI assistants can use to help humans with all aspects of ai-agent: configuration, development, operations, troubleshooting.

## Phase 1: Merge Skill Files
- [x] Read all three source files
- [x] Identify unique content in each
- [x] Create merged `ai-agent-guide.md`
- [x] Delete redundant source files

## Phase 2: Wiki Page Review - COMPLETED

Review each wiki page to extract content that should be in the skill. Mark each as:
- **DONE** - Content already covered in skill
- **ADD** - Content added to skill
- **SKIP** - Not relevant for operational/configuration skill

### Getting Started
- [x] 1. Home.md - DONE (overview content)
- [x] 2. Getting-Started.md - DONE (navigation)
- [x] 3. Getting-Started-Installation.md - DONE (git clone + build-and-install.sh)
- [x] 4. Getting-Started-Quick-Start.md - DONE (minimal examples)
- [x] 5. Getting-Started-First-Agent.md - DONE (agent structure)
- [x] 6. Getting-Started-Environment-Variables.md - DONE (env vars section)

### Agent Files
- [x] 7. Agent-Files.md - DONE (file structure)
- [x] 8. Agent-Files-Identity.md - DONE (toolName, description)
- [x] 9. Agent-Files-Models.md - DONE (models frontmatter)
- [x] 10. Agent-Files-Tools.md - DONE (tool naming, toolsAllowed/Denied clarification)
- [x] 11. Agent-Files-Sub-Agents.md - DONE (sub-agent patterns)
- [x] 12. Agent-Files-Behavior.md - DONE (behavior frontmatter)
- [x] 13. Agent-Files-Orchestration.md - DONE (advisors, router, handoff)
- [x] 14. Agent-Files-Contracts.md - DONE (input/output contracts)

### Agent Development
- [x] 15. Agent-Development.md - DONE (navigation)
- [x] 16. Agent-Development-Safety.md - DONE (safety gates section)
- [x] 17. Agent-Development-Multi-Agent.md - DONE (multi-agent patterns)

### CLI
- [x] 18. CLI.md - DONE (CLI reference)
- [x] 19. CLI-Running-Agents.md - DONE (invocation patterns)
- [x] 20. CLI-Overrides.md - DONE (--override keys)
- [x] 21. CLI-Scripting.md - DONE (stdin, exit codes)
- [x] 22. CLI-Debugging.md - DONE (debug commands)

### Configuration
- [x] 23. Configuration.md - DONE (navigation)
- [x] 24. Configuration-Files.md - DONE (file structure, resolution order)
- [x] 25. Configuration-Providers.md - DONE (provider examples)
- [x] 26. Configuration-MCP-Servers.md - DONE (MCP server config)
- [x] 27. Configuration-REST-Tools.md - DONE (REST tools, OpenAPI)
- [x] 28. Configuration-Tool-Filtering.md - DONE (toolsAllowed/Denied at MCP level)
- [x] 29. Configuration-Parameters.md - DONE (defaults section)
- [x] 30. Configuration-Pricing.md - DONE (pricing section)
- [x] 31. Configuration-Context-Window.md - DONE (context window)
- [x] 32. Configuration-Caching.md - DONE (cache config)
- [x] 33. Configuration-Queues.md - DONE (queues config)

### Headends
- [x] 34. Headends.md - DONE (headends table)
- [x] 35. Headends-REST.md - DONE (API headend)
- [x] 36. Headends-MCP.md - ADD (MCP headend requirements)
- [x] 37. Headends-OpenAI-Compatible.md - ADD (model naming convention)
- [x] 38. Headends-Anthropic-Compatible.md - ADD (model naming convention)
- [x] 39. Headends-Embed.md - DONE (embed headend)
- [x] 40. Headends-Library.md - ADD (library embedding example)
- [x] 41. Headends-Slack.md - DONE (Slack config)

### Operations
- [x] 42. Operations.md - DONE (navigation)
- [x] 43. Operations-Debugging.md - ADD (debug commands section)
- [x] 44. Operations-Logging.md - ADD (log severity levels table)
- [x] 45. Operations-Telemetry.md - DONE (telemetry config)
- [x] 46. Operations-Accounting.md - DONE (accounting config)
- [x] 47. Operations-Snapshots.md - DONE (snapshot location/format)
- [x] 48. Operations-Tool-Output.md - DONE (tool_output handling)
- [x] 49. Operations-Exit-Codes.md - ADD (CLI exit codes table)
- [x] 50. Operations-Troubleshooting.md - DONE (troubleshooting section)

### System Prompts
- [x] 51. System-Prompts.md - DONE (navigation)
- [x] 52. System-Prompts-Writing.md - DONE (prompt examples)
- [x] 53. System-Prompts-Variables.md - DONE (variables table)
- [x] 54. System-Prompts-Includes.md - ADD (corrected .env blocking note, FORMAT values table)

### Technical Specs
- [x] 55. Technical-Specs.md - SKIP (navigation)
- [x] 56. Technical-Specs-Index.md - SKIP (navigation)
- [x] 57. Technical-Specs-Architecture.md - SKIP (implementation detail)
- [x] 58. Technical-Specs-Design-History.md - SKIP (ADR history)
- [x] 59. Technical-Specs-Session-Lifecycle.md - SKIP (implementation detail)
- [x] 60. Technical-Specs-Tool-System.md - ADD (internal tools table expanded)
- [x] 61. Technical-Specs-Context-Management.md - SKIP (implementation detail)
- [x] 62. Technical-Specs-Retry-Strategy.md - ADD (maxRetries semantics clarification)
- [x] 63. Technical-Specs-User-Contract.md - SKIP (guarantees, implementation detail)

### Advanced
- [x] 64. Advanced.md - SKIP (navigation)
- [x] 65. Advanced-Override-Keys.md - ADD (override keys for testing section)
- [x] 66. Advanced-Internal-API.md - DONE (library embedding covered)
- [x] 67. Advanced-Hidden-CLI.md - DONE (advisors/handoff covered)
- [x] 68. Advanced-Extended-Reasoning.md - ADD (reasoning levels table, inheritance rules)

### Contributing
- [x] 69. Contributing.md - SKIP (not operational)
- [x] 70. Contributing-Documentation.md - SKIP (not operational)
- [x] 71. Contributing-Testing.md - SKIP (not operational)
- [x] 72. Contributing-Code-Style.md - SKIP (not operational)

### Utility
- [x] 73. _Sidebar.md - SKIP (navigation)
- [x] 74. USER-DOCS-STANDARD.md - SKIP (doc standards)

## Content Added from Wiki Review

The following content was added to `ai-agent-guide.md` based on wiki review:

1. **MCP Headend Requirements** - format and schema arguments
2. **OpenAI/Anthropic model naming** - dash vs underscore for deduplication
3. **Library Embedding example** - AIAgent.create() and AIAgent.run() pattern
4. **Log severity levels table** - VRB, WRN, ERR, TRC, THK, FIN
5. **Debug commands section** - dry-run, verbose, trace flags
6. **CLI exit codes table** - expanded with categories
7. **Include directive correction** - only files named exactly `.env` blocked
8. **FORMAT values table** - context-specific expansion
9. **Internal tools table expanded** - parameters and behavior
10. **maxRetries semantics** - 3 = 3 total attempts, not 3 retries
11. **Reasoning levels table** - budget calculations
12. **Reasoning inheritance rules** - sub-agent behavior
13. **Override keys for testing** - key override keys section

## Content Added from Parallel Sub-Agent Review (Round 2)

A second review was performed using 15 parallel sub-agents (5 wiki pages each):

14. **Windows WSL note** - Installation section clarification
15. **Reserved tool names** - agent__ prefix is reserved
16. **--default-reasoning flag** - CLI options and reasoning section
17. **persistence.billingFile** - Updated from deprecated accounting.file
18. **Unknown variables behavior** - Left unchanged, not error
19. **Include error messages** - Missing file, circular, depth errors
20. **jq analysis queries** - Snapshot extraction commands
21. **Tokenizer config** - Provider/model level tokenizer option
22. **CLI-only variables clarification** - Only in inline prompts
23. **REST streaming config** - streaming: true option
24. **Heredoc CLI example** - Multi-line prompt pattern
25. **Model fallback behavior** - Models tried in order
26. **output vs expectedOutput** - Clarified difference

## Status: COMPLETED

All 74 wiki pages have been reviewed in two passes. The skill document `docs/skills/ai-agent-guide.md` is now comprehensive and ready for use.
