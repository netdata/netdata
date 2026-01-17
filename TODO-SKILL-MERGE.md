# AI Agent Skill Document Merge

## TL;DR
Merge three skill files (`ai-agent-configuration.md`, `ai-agent-configuration-user.md`, `ai-agent-user-guide.md`) into a single comprehensive `ai-agent-guide.md` skill document. Then review all wiki pages to enrich the skill with any missing content.

## Objective
Create a single, dense, comprehensive skill document that AI assistants can use to help humans with all aspects of ai-agent: configuration, development, operations, troubleshooting.

## Phase 1: Merge Skill Files
- [x] Read all three source files
- [x] Identify unique content in each
- [x] Create merged `ai-agent-guide.md`
- [ ] Delete redundant source files (after user approval)

## Phase 2: Wiki Page Review

Review each wiki page to extract content that should be in the skill. Mark each as:
- **DONE** - Content already covered in skill
- **ADD** - Content needs to be added to skill
- **SKIP** - Not relevant for operational/configuration skill

### Getting Started
- [ ] 1. Home.md
- [ ] 2. Getting-Started.md
- [ ] 3. Getting-Started-Installation.md
- [ ] 4. Getting-Started-Quick-Start.md
- [ ] 5. Getting-Started-First-Agent.md
- [ ] 6. Getting-Started-Environment-Variables.md

### Agent Files
- [ ] 7. Agent-Files.md
- [ ] 8. Agent-Files-Identity.md
- [ ] 9. Agent-Files-Models.md
- [ ] 10. Agent-Files-Tools.md
- [ ] 11. Agent-Files-Sub-Agents.md
- [ ] 12. Agent-Files-Behavior.md
- [ ] 13. Agent-Files-Orchestration.md
- [ ] 14. Agent-Files-Contracts.md

### Agent Development
- [ ] 15. Agent-Development.md
- [ ] 16. Agent-Development-Safety.md
- [ ] 17. Agent-Development-Multi-Agent.md

### CLI
- [ ] 18. CLI.md
- [ ] 19. CLI-Running-Agents.md
- [ ] 20. CLI-Overrides.md
- [ ] 21. CLI-Scripting.md
- [ ] 22. CLI-Debugging.md

### Configuration
- [ ] 23. Configuration.md
- [ ] 24. Configuration-Files.md
- [ ] 25. Configuration-Providers.md
- [ ] 26. Configuration-MCP-Servers.md
- [ ] 27. Configuration-REST-Tools.md
- [ ] 28. Configuration-Tool-Filtering.md
- [ ] 29. Configuration-Parameters.md
- [ ] 30. Configuration-Pricing.md
- [ ] 31. Configuration-Context-Window.md
- [ ] 32. Configuration-Caching.md
- [ ] 33. Configuration-Queues.md

### Headends
- [ ] 34. Headends.md
- [ ] 35. Headends-REST.md
- [ ] 36. Headends-MCP.md
- [ ] 37. Headends-OpenAI-Compatible.md
- [ ] 38. Headends-Anthropic-Compatible.md
- [ ] 39. Headends-Embed.md
- [ ] 40. Headends-Library.md
- [ ] 41. Headends-Slack.md

### Operations
- [ ] 42. Operations.md
- [ ] 43. Operations-Debugging.md
- [ ] 44. Operations-Logging.md
- [ ] 45. Operations-Telemetry.md
- [ ] 46. Operations-Accounting.md
- [ ] 47. Operations-Snapshots.md
- [ ] 48. Operations-Tool-Output.md
- [ ] 49. Operations-Exit-Codes.md
- [ ] 50. Operations-Troubleshooting.md

### System Prompts
- [ ] 51. System-Prompts.md
- [ ] 52. System-Prompts-Writing.md
- [ ] 53. System-Prompts-Variables.md
- [ ] 54. System-Prompts-Includes.md

### Technical Specs
- [ ] 55. Technical-Specs.md
- [ ] 56. Technical-Specs-Index.md
- [ ] 57. Technical-Specs-Architecture.md
- [ ] 58. Technical-Specs-Design-History.md
- [ ] 59. Technical-Specs-Session-Lifecycle.md
- [ ] 60. Technical-Specs-Tool-System.md
- [ ] 61. Technical-Specs-Context-Management.md
- [ ] 62. Technical-Specs-Retry-Strategy.md
- [ ] 63. Technical-Specs-User-Contract.md

### Advanced
- [ ] 64. Advanced.md
- [ ] 65. Advanced-Override-Keys.md
- [ ] 66. Advanced-Internal-API.md
- [ ] 67. Advanced-Hidden-CLI.md
- [ ] 68. Advanced-Extended-Reasoning.md

### Contributing
- [ ] 69. Contributing.md
- [ ] 70. Contributing-Documentation.md
- [ ] 71. Contributing-Testing.md
- [ ] 72. Contributing-Code-Style.md

### Utility
- [ ] 73. _Sidebar.md (navigation, skip)
- [ ] 74. USER-DOCS-STANDARD.md (doc standards, skip)

## Decisions Made
1. Merge all three files into one comprehensive skill
2. Use dense table format for configuration options
3. Keep operational behavior explanations for AI understanding
4. Include troubleshooting patterns

## Known Issues to Fix in Merge
1. `toolsAllowed`/`toolsDenied` in frontmatter - NOT supported (only in MCP server config)
2. REST tools in frontmatter use config key name (e.g., `weather`), not `rest__weather`
3. Some default values differ between files (e.g., `maxRetries` 3 vs 5) - verify from source

## Source Files
- `docs/skills/ai-agent-configuration.md` (642 lines) - AI-facing technical spec
- `docs/skills/ai-agent-configuration-user.md` (891 lines) - Configuration reference
- `docs/skills/ai-agent-user-guide.md` (832 lines) - User guide with tutorial

## Target File
- `docs/skills/ai-agent-guide.md` - Single comprehensive skill document
