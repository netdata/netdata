# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

THE MOST IMPORTANT RULES ARE:

1. Why always MUST find the root cause of a problem.
2. Patching without understanding a problem IS NOT ALLOWED.
3. Before we take any action, we MUST understand the code base and the implications.
4. We do not duplicate code. Check if similar code already exists and reuse.

## MCP Web Client Settings Architecture

### Per-Chat Isolation
- **CRITICAL**: Each chat has its own isolated settings and MessageOptimizer instance
- NO global sharing of settings between chats
- Each chat maintains its own optimizer configuration independently

### Default Behavior
- All optimization features are DISABLED by default
- Only the primary model is set (no secondary model by default)
- Users must explicitly enable each optimization feature

### Settings Persistence
- When a user changes settings in any chat, they are saved to localStorage as 'lastChatConfig'
- New chats inherit settings from the last saved configuration
- This includes: model selection, MCP server, LLM provider, and optimizer settings

### MessageOptimizer Integration
- Each chat creates its own MessageOptimizer instance with isolated settings
- The optimizer handles all message filtering, transformation, and cost optimization
- Settings structure:
  ```javascript
  {
    primaryModel: string,      // Required
    secondaryModel: string,    // Optional, null by default
    toolSummarization: {
      enabled: false,          // Disabled by default
      threshold: 50000,
      useSecondaryModel: true
    },
    toolMemory: {
      enabled: false,          // Disabled by default
      forgetAfterConclusions: 1
    },
    cacheControl: {
      enabled: false,          // Disabled by default
      strategy: 'smart'
    },
    autoSummarization: {
      enabled: false,          // Disabled by default
      triggerPercent: 50,
      useSecondaryModel: true
    }
  }
  ```

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
