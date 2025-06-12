# ASSISTANTS **MUST** FOLLOW THESE RULES

1. This is a new application. No need for backward compatibility.
2. When a task is concluded existing eslint configuration MUST show zero errors and zero warnings.
3. When migrating/refactoring code, NEVER use fallbacks. Let the code fail so that we can identify and fix issues immediately.
4. FAIL FAST strategy, not fallback silently.
5. Do not keep code for future use. If you need to implement a feature later, do it then.
6. Do not use any deprecated or legacy code. This is a new application, so everything should be up-to-date.
7. Avoid default values to parameters. Always call functions with explicit parameters and if adding new parameters, ensure all uses are updated accordingly.
8. Remove any unused code, including imports, variables, functions, etc. If you need to use it later, re-implement it.

## WHEN COMMITING
1. Commit with git add filename1 filename2 etc, to avoid committing unnecessary files.
2. Always do a git diff before committing to ensure only intended changes are included.

---

# MCP Web Client - Chat History Implementation

This document describes the chat history implementation and message handling logic in the MCP web client.

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

### Cache Position Tracking
- Each assistant message stores `cacheControlIndex` indicating where cache control was applied
- This allows freezing the cache position for cost-effective operations

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

## Tool Inclusion Modes

The `toolInclusionMode` property controls how tools are included:
- `auto` - Automatic inclusion based on context
- `cached` - Always include tools with cache control (default)
- `all-on` - Include all tools
- `all-off` - Exclude all tools
- `manual` - User controls individual tool inclusion

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