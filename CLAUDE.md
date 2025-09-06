# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

THE MOST IMPORTANT RULES ARE:

1. You MUST ALWAYS find the root cause of a problem, before giving a solution.
2. Patching without understanding the problem IS NOT ALLOWED.
3. Before patching code, we MUST understand the code base and the potential implications of our changes.
4. We do not duplicate code. We first check if similar code already exists and to reuse it.

## Collector Consistency Requirements

When working on collectors (especially Go collectors), ALL of the following files MUST be kept in sync before creating a PR:

1. **The code** - All .go files implementing the collector
2. **metadata.yaml** - Proper information for the Netdata integrations page, including:
   - Metric descriptions with correct units
   - Alert definitions
   - Setup instructions
   - Configuration examples
3. **config_schema.json** - Schema for dynamic configuration in the dashboard
4. **Stock config file** (.conf file) - Example configuration users edit manually
5. **Health alerts** (health.d/*.conf) - Alert definitions for the collector metrics
6. **README.md** - Comprehensive documentation describing:
   - What the collector monitors
   - How it works
   - Configuration options
   - Troubleshooting

These files MUST be consistent with each other. For example:
- If units change in code, they MUST be updated in metadata.yaml
- If new metrics are added, they MUST be documented in metadata.yaml and README.md
- If configuration options change, they MUST be updated in config_schema.json, stock config, and documentation

## C code
- gcc, clang, glibc and muslc
- libnetdata.h includes everything in libnetdata (just a couple of exceptions) so there is no need to include individual libnetdata headers
- Functions with 'z' suffix (mallocz, reallocz, callocz, strdupz, etc.) handle allocation failures automatically by calling fatal() to exit Netdata
- The freez() function accepts NULL pointers without crashing
- Resuable, generic, module agnostic code, goes to libnetdata
- Double linked lists are managed with DOUBLE_LINKED_LIST_* macros
- json-c for json parsing
- buffer_json_* for manual json generation

## Naming Conventions
- "Netdata Agent" (capitalized) when referring to the product
- "`netdata`" (lowercase, code-formatted) when referring to the process
- See DICTIONARY.md for precise terminology
