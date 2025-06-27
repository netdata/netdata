# ASSISTANTS **MUST** FOLLOW THESE RULES

## 🚨 PRINCIPLE-001: ERROR VISIBILITY AND FAILURE HANDLING 🚨

**CRITICAL RULE: WHEN WRITING CODE WE REVEAL UNEXPECTED ERRORS. WE DON'T WORK AROUND THEM.**

- **UNEXPECTED ERRORS MUST BE PROMINENT** - Make them visible so developers can see and fix them
- **SILENT ERROR HANDLING IS NEVER PERMITTED** - Unless it's expected behavior in normal processing flow
- **UNEXPECTED ERRORS MUST BE LOGGED** - For developers to see (console.error)
- **ERRORS AFFECTING USER FLOW MUST BE SHOWN ON UI** - Users must be informed when something fails
- **ANY CODE NOT COMPLYING IS INCORRECT AND MUST BE FIXED IMMEDIATELY**

**JavaScript developers have the BAD habit of providing fallbacks and default values on UNEXPECTED things. This is a SEVERE FLAW and IS NOT ACCEPTED in this project.**

**Every fallback, default, and workaround MUST HAVE SPECIFIC BUSINESS LOGIC REASONING, otherwise it MUST be immediately fixed to log and FAIL.**

**Examples of INCORRECT patterns:**
```javascript
// WRONG - Silent failure
const chat = this.chats.get(chatId) || {};

// WRONG - Fallback without reasoning
const result = data?.property || 'default';

// CORRECT - Explicit error handling
const chat = this.chats.get(chatId);
if (!chat) {
    console.error(`[functionName] Chat not found for chatId: ${chatId}`);
    return; // or throw, or show UI error
}
```

1. This is a new application. No need for backward compatibility.
2. When a task is concluded, existing `eslint` configuration MUST show zero errors and zero warnings.
3. Do not use any deprecated or legacy code. This is a new application, so everything should be up-to-date.
4. Avoid default values to parameters. Always call functions with explicit parameters and if adding new parameters, ensure all uses are updated accordingly.
5. Work in small increments so that the user can follow up with your changes and provide feedback.
6. When dealing with issues, ALWAYS FIND THE ROOT CAUSE and fix it, rather than applying a workaround.

## REFACTORING
1. When migrating/refactoring code, NEVER use fallbacks. Let the code fail so that we can identify and fix issues immediately.
2. Do not keep code for future use. If you need to implement a feature later, do it then.
3. FAIL FAST strategy, not fallback silently.
4. Remove any unused code, including imports, variables, functions, etc. If you need to use it later, re-implement it.

## WHEN COMMITING
1. BEFORE committing make sure `eslint` shows zero errors and zero warnings.
1. Commit with git add filename1 filename2 etc, to avoid committing unnecessary files.
2. Always do a git diff before committing to ensure only intended changes are included.

NOTES:
 - Run `npx eslint ...` not `npm run lint`.

---

# MCP Web Client - Chat History Implementation

This document describes the chat history implementation and message handling logic in the MCP web client.

IMPORTANT:
Each chat in this application is INDEPENDENT OF THE OTHERS. There shouldn't be ANY global configuration of ANY kind.
Each chat has its own UNIQUE configuration, its own messages and data, and chats run IN PARALLEL. So, the user is able
to SWITCH BETWEEN THEM AT ANY TIME without any issues, and let them run while he chats on another one of them.

CRITICAL:
No sharing between chats. No global settings. No shared structures.

CRITICAL:
NO HARDCODING OF PROVIDERS AND MODELS ALLOWED! The providers and models are coming from llm-proxy.js and are USER CONFIGURABLE.
Even llm-proxy.js itself has an internal list only to help the user configure the providers and models.
So, the application MUST NOT hardcode any providers or models. It should use the ones configured by the user.

## Message Roles and Types

The chat history uses a role-based system to distinguish different types of messages:

### Regular Conversation Roles
- `user` - Regular user messages
- `assistant` - LLM assistant responses
- `system` - System prompt (only sent at the beginning of conversations)

### Special System Roles (Never sent to LLM API)
- `system-title` - Request to generate a chat title
- `title` - Response containing the generated title
- `system-summary` - Request to summarize the conversation
- `summary` - Response containing the conversation summary (acts as checkpoint)
- `accounting` - Token accounting checkpoint (preserves cumulative token counts)

### Legacy Types (for backward compatibility)
- `tool-results` - Results from tool executions
- `tool-call` - Tool invocation records

## Visual Styling

Each role has distinct visual styling:
- **system-title/system-summary**: Border with icon (📝 for title, 📋 for summary)
- **title**: Italicized with blue border
- **summary**: Yellow border with "📌 Conversation Summary (Checkpoint)" header
- **accounting**: Horizontal line with token counts (💰 icon)
- **Regular messages**: Standard chat bubbles

## API Message Building (`buildMessagesForAPI`)

When building messages for the LLM API:

1. **System roles are filtered out**: Messages with roles `system-title`, `system-summary`, `title`, `summary`, and `accounting` are never sent to the API

2. **Summary checkpoint behavior**:
   - Finds the latest `summary` message
   - Only includes messages AFTER the summary
   - The summary content is prepended to the system prompt as "Previous Conversation Summary:"
   - This prevents sending the entire conversation history after summarization

3. **System prompt inclusion**: Added when it's the first user message in the conversation or after a checkpoint

## Cache Control Management

### Cache Control Modes
The cache control system now uses a simple 3-value configuration:

- **`all-off`**: No cache control headers are emitted (default for new chats)
- **`system`**: Cache control is applied ONLY to the system prompt
- **`cached`**: Smart cache control - caches system prompt AND applies message-level caching

### Configuration Changes
- **Old Format**: `{ enabled: boolean, strategy: string }`
- **New Format**: Single string value (`'all-off'`, `'system'`, `'cached'`)
- **Migration**: Old configs are automatically migrated on load
- **UI**: Dropdown selection instead of checkbox + mutual exclusivity

### Cache Position Tracking
- Each assistant message stores `cacheControlIndex` indicating where cache control was applied
- This allows freezing the cache position for cost-effective operations

### System Prompt Caching
- **`system` mode**: Only the system prompt gets cache control headers
- **`cached` mode**: System prompt + message-level smart caching
- **`all-off` mode**: No caching anywhere

### Message-Level Caching (cached mode only)
- Uses smart strategy: caches up to 70% of messages, avoiding recent tool results
- Can be frozen during summary operations to prevent cache creation surcharge

### Frozen Cache for Summaries
- When requesting a summary, `buildMessagesForAPI(chat, provider, true)` freezes the cache
- The cache control mark stays at its previous position instead of advancing
- This prevents the 25% cache creation surcharge on the entire conversation

## Context Window Calculation

### Token Components
The context window includes:
- `promptTokens` - Regular input tokens
- `cacheReadInputTokens` - Cached tokens being reused (90% discount)
- `cacheCreationInputTokens` - New tokens being cached (25% surcharge)
- `completionTokens` - Output tokens (will be part of next request's input)

### Special Handling by Role
- **Regular messages**: Full token calculation including all components
- **system-title, title, system-summary**: Maintains current context window (no change)
- **summary**: Returns only the summary's completion tokens (context is reset)

### Why Include Completion Tokens
The assistant's response (completion tokens) becomes part of the conversation history sent in the next request, so they must be counted as part of the context.

## Tool Filtering

Tool filtering is now handled exclusively by the Message Optimizer:
- **Single Source of Truth**: The Message Optimizer determines which tools to include
- **No Provider Filtering**: LLM providers no longer have tool filtering logic
- **Context-Aware**: Tools are filtered based on Tool Memory settings and conversation context
- **Automatic**: No manual tool inclusion modes - all handled transparently

### Removed Concepts
- `toolInclusionMode` parameter is no longer used in providers
- Manual tool filtering has been removed in favor of automatic optimization
- All tool filtering logic consolidated in the Message Optimizer

## Summary Workflow

1. User clicks "Summarize Conversation"
2. System sends `system-summary` request with frozen cache
3. LLM responds with summary
4. Summary is stored with role `summary` and acts as checkpoint
5. Context window resets to show only summary tokens
6. Future API calls include summary in system prompt but exclude all prior messages

## Title Generation Workflow

1. System sends `system-title` request
2. LLM responds with title
3. Both messages are stored but never sent in future API calls
4. Context window remains unchanged throughout the process

## Important Implementation Notes

1. **Message Filtering**: The `buildMessagesForAPI` function is the single source of truth for what gets sent to the LLM
2. **Context Calculation**: The `calculateContextWindowTokens` function handles special cases based on message roles
3. **Cache Efficiency**: Summary requests don't advance the cache, saving costs on large conversations
4. **Checkpoint Behavior**: Summaries create a hard boundary - messages before them are never sent again

## Live vs Loaded Chats - Unified Architecture

**Critical Design Principle**: Both live and loaded chats use identical mechanics. The chat history stores only raw data - no pre-calculated statistics.

### Why No Stored Statistics
- All statistics (token counts, context window size, etc.) are calculated on-the-fly
- This ensures consistency between live and loaded chats
- Prevents stale or incorrect statistics from persisting
- Simplifies the data model - chat history contains only raw messages and responses

### Unified Processing
- `convertMessageToEvents()` transforms stored messages into rendering events
- The same rendering pipeline handles both live and historical messages
- Token counting, context calculation, and display updates work identically

## Context Window Calculation Details

The context window is calculated dynamically by `calculateContextWindowTokens()`:

### For Regular Messages (user/assistant)
```
Context = promptTokens + cacheReadInputTokens + cacheCreationInputTokens + completionTokens
```
- Includes all token types from the latest request
- Completion tokens are counted because they'll be sent in the next request

### For System Title Messages (system-title/title)
- **No change to context window**
- Returns the current context size unchanged
- These messages are UI-only and don't affect the conversation flow

### For System Summary Messages  
- **system-summary request**: No change (maintains current context)
- **summary response**: Context resets to only the summary's completion tokens
- This reflects that future requests will only include the summary

### Context Window After Operations
1. **Normal conversation**: Accumulates tokens with each exchange
2. **After title generation**: Unchanged from before
3. **After summarization**: Shows only summary tokens (typically much smaller)
4. **Loading a chat**: Recalculates from the latest token history entry

### Token History Storage
- Raw token usage is stored per request in `tokenUsageHistory`
- Each entry contains: `promptTokens`, `completionTokens`, `cacheReadInputTokens`, `cacheCreationInputTokens`
- No derived values or totals are stored - everything is calculated when needed

## Accounting Nodes - Preserving Token History

### Purpose
When users edit messages or retry after errors, messages are removed from the chat history. This would normally cause the cumulative token counters to lose track of the actual costs incurred. Accounting nodes solve this by creating checkpoints that preserve the token counts before messages are discarded.

### Structure
```javascript
{
  role: 'accounting',
  timestamp: '...',
  cumulativeTokens: {
    inputTokens: 12500,
    outputTokens: 8300,
    cacheReadTokens: 5000,
    cacheCreationTokens: 1200
  },
  reason: 'Message edited', // or 'Retry after error'
  discardedMessages: 5 // Number of messages removed
}
```

### When Created
1. **Before message editing** - When a user edits a message, all subsequent messages are removed
2. **Before retry operations** - When retrying after an error, error and subsequent messages are removed

### Visual Representation
- Displayed as a horizontal line with token counts
- Shows 💰 icon followed by cumulative tokens at that point
- Includes the reason for the checkpoint

### Impact on Cumulative Tokens
The `getCumulativeTokenUsage()` function:
1. Finds the last accounting node in the chat history
2. Starts with its cumulative totals
3. Adds only the token usage from messages after that accounting node
4. This ensures the true total cost is always preserved

### Important Notes
- Accounting nodes are never sent to the LLM API
- They only affect cumulative token display, not context window calculation
- Multiple accounting nodes can exist in a chat history
- They provide an audit trail of conversation edits and retries

## Tool Memory - Rolling Window

### Overview
Tool Memory implements a rolling window for tool visibility. Tools and their responses are filtered based on how many "turns" have passed since they were used.

### What is a Turn?
A **turn** is a period of assistant activity that ends with a conclusion - an assistant message without tool calls. When the assistant sends a message without using tools, it marks the end of the current turn.

### Configuration
- `toolMemory.enabled`: Enable/disable the feature (default: false)
- `toolMemory.forgetAfterConclusions`: Number of turns to keep tools visible (0-5, default: 1)

### How It Works

1. **Turn Tracking**: Each message is assigned to a turn number based on when it occurs
2. **Turn Progression**: A new turn starts after the assistant concludes (sends a message without tools)
3. **Filtering**: Tools are filtered if `(currentTurn - toolTurn) > forgetAfterConclusions`

### Examples

#### With `forgetAfterConclusions = 0` (immediate filtering):
```
Turn 0: Assistant uses tools A,B → concludes
Turn 1: Assistant uses tools C,D → concludes
Result: When in Turn 1, tools A,B are filtered (no longer visible)
```

#### With `forgetAfterConclusions = 1` (keep 1 turn):
```
Turn 0: Assistant uses tools A,B → concludes
Turn 1: Assistant uses tools C,D → concludes  
Turn 2: Assistant uses tools E,F → concludes
Result: When in Turn 2, tools A,B are filtered, but C,D are still visible
```

### Implementation Details

1. **Two-Pass Processing**:
   - First pass: Build the turn map by analyzing all messages
   - Second pass: Filter messages based on turn age

2. **Complete Filtering**:
   - Tool response messages (`tool-results`) are removed entirely
   - Tool calls in assistant messages (`tool_use` blocks) are also removed
   - This prevents the assistant from seeing orphaned tool calls without responses

3. **User Messages**:
   - User messages don't create new turns
   - They reset the "hasToolsInCurrentTurn" flag but preserve turn counting

### UI Integration
- Checkbox to enable/disable tool memory
- Dropdown to select threshold (0-5 turns)
- Tooltip shows human-readable text:
  - 0 = "forget immediately"
  - 1 = "forget after 1 turn"
  - 2+ = "forget after N turns"
  - Disabled = "Always remember"

### Purpose
This feature helps manage context window size and reduces costs by automatically removing old tool interactions that are no longer relevant to the current conversation flow.

### Cache Control Interaction
Tool Memory and Cache Control can now be used together:
- **Independent Features**: Tool Memory filtering and Cache Control are separate optimizations
- **No Mutual Exclusivity**: Users can enable both features simultaneously
- **Smart Optimization**: The Message Optimizer handles both features intelligently
- **Cost Efficiency**: Tool Memory reduces context size, Cache Control reduces repeated processing