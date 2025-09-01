# Slack Bot Integration Capabilities

## Required Libraries and Versions

### Primary Library: @slack/bolt
- **Latest Version**: 3.22.0 (as of December 2024)
- **Installation**: `npm install @slack/bolt`
- **Node.js Requirement**: >= 12.13.0 (recommended: >= 20.0.0)
- **Documentation**: https://slack.dev/bolt-js/

### Related Dependencies
```json
{
  "@slack/bolt": "^3.22.0",
  "@slack/web-api": "^7.8.0",  // Included with bolt
  "@slack/oauth": "^3.0.0",     // For OAuth handling
  "@slack/socket-mode": "^2.0.0" // For Socket Mode connections
}
```

### Installation Commands
```bash
# Install latest stable version
npm install @slack/bolt

# For TypeScript support
npm install --save-dev @types/node

# For Socket Mode (recommended for development)
npm install @slack/socket-mode
```

## Overview
This document outlines the full capabilities of a Slack bot integration for the AI agent, including triggering events, messaging features, progress indicators, and interactive elements.

## Event Triggers - When Bot Can React

### Direct Triggers
- **`app_mention`** - When someone mentions the bot with `@bot-name` in any channel
- **`message.im`** - Direct messages sent to the bot
- **`message.channels`** - Every message in channels where bot is a member (requires explicit permission)
- **`message.groups`** - Messages in private channels where bot is a member
- **`message.mpim`** - Multi-person direct messages including the bot

### Thread & Conversation Events
- **`message.threads`** - Replies within threads
- **Thread continuations** - Can maintain context within a thread conversation
- **Message replies** - Direct replies to bot's messages

### Interactive Triggers
- **Slash commands** - Custom commands like `/ai-agent query database`
- **Shortcuts** - Global or message shortcuts from Slack UI
- **Button clicks** - Response to interactive button elements
- **Select menus** - Dropdown selection interactions
- **Modal submissions** - Form submissions from modal dialogs

### Reaction Events
- **`reaction_added`** - When users add emoji reactions
- **`reaction_removed`** - When users remove reactions
- Can use reactions as commands (e.g., 🔄 for regenerate, ❌ for cancel)

### Workspace Events
- **`member_joined_channel`** - When bot joins a channel
- **`app_home_opened`** - When user opens bot's App Home tab
- **File uploads** - Can process files shared with the bot
- **Scheduled messages** - Time-based triggers

## Message Posting Capabilities

### Where Bot Can Post

#### Channels
- Any public channel where bot is a member
- Private channels where bot is invited
- Can be invited with `/invite @bot-name`

#### Direct Messages
- Can initiate DMs with any workspace member
- Multi-person DMs when included

#### Threads
- Reply to any message creating a thread
- Continue existing thread conversations
- Maintain thread context using `thread_ts`

### Message Types

#### Basic Messages
```javascript
await client.chat.postMessage({
  channel: 'C1234567890',
  text: 'Hello, world!'
});
```

#### Rich Text Formatting
- **Markdown support**: Bold, italic, strikethrough, code
- **Code blocks**: With syntax highlighting
- **Links**: Automatic URL detection and custom link text
- **User mentions**: `<@U1234567890>`
- **Channel references**: `<#C1234567890>`
- **Emoji**: Standard and custom workspace emoji

#### Block Kit UI Components
```javascript
blocks: [
  {
    type: "section",
    text: {
      type: "mrkdwn",
      text: "*AI Response*\nHere's what I found..."
    }
  },
  {
    type: "divider"
  },
  {
    type: "actions",
    elements: [
      {
        type: "button",
        text: { type: "plain_text", text: "Regenerate" },
        action_id: "regenerate"
      }
    ]
  }
]
```

## Progress Indicators

### Live Message Updates
The bot can update messages in real-time to show progress:

```javascript
// Initial message
const message = await client.chat.postMessage({
  channel: event.channel,
  text: "🔍 Starting analysis..."
});

// Update 1 - After 2 seconds
await client.chat.update({
  channel: event.channel,
  ts: message.ts,
  text: "📊 Processing data... (25%)"
});

// Update 2 - After 4 seconds
await client.chat.update({
  channel: event.channel,
  ts: message.ts,
  text: "🧠 Running AI model... (50%)"
});

// Update 3 - After 6 seconds
await client.chat.update({
  channel: event.channel,
  ts: message.ts,
  text: "✍️ Generating response... (75%)"
});

// Final update
await client.chat.update({
  channel: event.channel,
  ts: message.ts,
  text: "✅ Complete! Here's your answer:\n\n[Full response...]"
});
```

### Progress Patterns

#### Pattern 1: Status Line Updates
```
🔄 Initializing...
🔍 Searching documentation...
📚 Found 15 relevant documents...
🧠 Analyzing with AI model...
✍️ Composing response...
✅ Complete!
```

#### Pattern 2: Detailed Progress Blocks
```javascript
blocks: [
  {
    type: "section",
    text: {
      type: "mrkdwn",
      text: "*Task Progress*"
    }
  },
  {
    type: "section",
    text: {
      type: "mrkdwn",
      text: "✅ Connected to AI\n✅ Query parsed\n⏳ Searching knowledge base...\n⏸️ Generating response\n⏸️ Formatting output"
    }
  },
  {
    type: "context",
    elements: [
      {
        type: "mrkdwn",
        text: "Elapsed: 3.2s | Tokens: 1,250"
      }
    ]
  }
]
```

#### Pattern 3: Streaming Responses
Stream the AI response character by character or chunk by chunk:

```javascript
let response = "";
const message = await client.chat.postMessage({
  channel: event.channel,
  text: "💭 Thinking..."
});

// As chunks arrive from AI
onChunk: async (chunk) => {
  response += chunk;
  // Update every 500ms or every 50 characters
  await client.chat.update({
    channel: event.channel,
    ts: message.ts,
    text: response + "▊" // Cursor indicator
  });
}
```

### Typing Indicators
Show "bot is typing..." indicator:

```javascript
// Shows typing indicator for ~3 seconds
await client.rtm.sendTyping({
  channel: event.channel
});
```

### Ephemeral Messages
Temporary messages only visible to one user:

```javascript
await client.chat.postEphemeral({
  channel: event.channel,
  user: event.user,
  text: "⏳ Processing your private request..."
});
```

## Interactive Elements

### Action Buttons
```javascript
{
  type: "actions",
  elements: [
    {
      type: "button",
      text: { type: "plain_text", text: "🔄 Regenerate" },
      style: "primary",
      action_id: "regenerate_response"
    },
    {
      type: "button",
      text: { type: "plain_text", text: "🎯 More Specific" },
      action_id: "more_specific"
    },
    {
      type: "button",
      text: { type: "plain_text", text: "📝 Simpler" },
      action_id: "simpler"
    },
    {
      type: "button",
      text: { type: "plain_text", text: "❌ Cancel" },
      style: "danger",
      action_id: "cancel"
    }
  ]
}
```

### Select Menus
```javascript
{
  type: "section",
  text: {
    type: "mrkdwn",
    text: "Choose AI model:"
  },
  accessory: {
    type: "static_select",
    placeholder: {
      type: "plain_text",
      text: "Select model"
    },
    options: [
      {
        text: { type: "plain_text", text: "GPT-4" },
        value: "gpt-4"
      },
      {
        text: { type: "plain_text", text: "Claude 3" },
        value: "claude-3"
      }
    ],
    action_id: "model_select"
  }
}
```

### Modal Dialogs
```javascript
await client.views.open({
  trigger_id: body.trigger_id,
  view: {
    type: "modal",
    title: { type: "plain_text", text: "AI Agent Settings" },
    blocks: [
      {
        type: "input",
        element: {
          type: "plain_text_input",
          multiline: true,
          action_id: "prompt_input"
        },
        label: { type: "plain_text", text: "System Prompt" }
      }
    ]
  }
});
```

## Advanced Features

### Context Preservation
- Maintain conversation context within threads
- Store user preferences per channel/user
- Remember previous queries in session

### File Handling
```javascript
// Respond to file uploads
app.event('file_shared', async ({ event, client }) => {
  const file = await client.files.info({ file: event.file_id });
  // Process file content with AI
});
```

### Scheduled Messages
```javascript
await client.chat.scheduleMessage({
  channel: 'C1234567890',
  post_at: Math.floor(Date.now() / 1000) + 3600, // 1 hour from now
  text: "Scheduled AI summary"
});
```

### Multi-Step Workflows
1. User triggers bot with command
2. Bot shows modal for additional input
3. Bot posts progress updates
4. Bot streams response
5. Bot adds interactive buttons for follow-up

## Rate Limits & Constraints

### API Rate Limits
- **`chat.postMessage`**: ~1 per second per channel
- **`chat.update`**: ~30 updates per minute per message
- **`views.open`**: ~50 per minute
- **`files.upload`**: ~20 per minute
- **Typing indicator**: Auto-expires after ~3 seconds

### Message Constraints
- **Text length**: 40,000 characters max
- **Blocks**: 50 blocks max per message
- **Attachments**: 100 attachments max
- **File uploads**: 1GB max file size
- **Thread replies**: No limit on thread depth

### Best Practices
1. **Immediate acknowledgment**: Always respond immediately, even if just "Processing..."
2. **Regular updates**: Update progress every 2-3 seconds for long operations
3. **Graceful degradation**: Fall back to simple text if blocks fail
4. **Error handling**: Always provide clear error messages
5. **Thread usage**: Use threads for conversations to avoid channel spam
6. **Rate limit handling**: Implement exponential backoff
7. **Timeout handling**: Set reasonable timeouts (30s max for user-facing operations)

## Basic Implementation Example with Bolt.js

### Minimal Setup
```javascript
const { App } = require('@slack/bolt');

const app = new App({
  token: process.env.SLACK_BOT_TOKEN,
  signingSecret: process.env.SLACK_SIGNING_SECRET,
  socketMode: true,  // Enable for development
  appToken: process.env.SLACK_APP_TOKEN  // Required for socket mode
});

// Listen for app mentions
app.event('app_mention', async ({ event, client, say }) => {
  // Post initial progress message
  const result = await client.chat.postMessage({
    channel: event.channel,
    thread_ts: event.thread_ts || event.ts,
    text: "🔍 Processing your request..."
  });

  // Simulate progress updates
  await client.chat.update({
    channel: event.channel,
    ts: result.ts,
    text: "🧠 Analyzing with AI..."
  });

  // Final response
  await client.chat.update({
    channel: event.channel,
    ts: result.ts,
    text: "✅ Here's your answer: [Response]"
  });
});

// Start the app
(async () => {
  await app.start();
  console.log('⚡️ Bolt app is running!');
})();
```

### TypeScript Implementation
```typescript
import { App, AppMentionEvent } from '@slack/bolt';

const app = new App({
  token: process.env.SLACK_BOT_TOKEN!,
  signingSecret: process.env.SLACK_SIGNING_SECRET!,
  socketMode: true,
  appToken: process.env.SLACK_APP_TOKEN!
});

app.event<'app_mention'>('app_mention', async ({ event, client }) => {
  const mentionEvent = event as AppMentionEvent;
  
  // Extract text after mention
  const text = mentionEvent.text.replace(/<@[A-Z0-9]+>/g, '').trim();
  
  // Post and update message
  const result = await client.chat.postMessage({
    channel: mentionEvent.channel,
    thread_ts: mentionEvent.thread_ts ?? mentionEvent.ts,
    text: '🤔 Processing...'
  });

  if (result.ts) {
    // Update with final response
    await client.chat.update({
      channel: mentionEvent.channel,
      ts: result.ts,
      text: `Response to: ${text}`
    });
  }
});

await app.start();
```

## Example Implementation Flow

### Complete User Interaction
```
1. User: @ai-agent explain quantum computing in simple terms

2. Bot: [Immediate] 
   🤔 Processing your request...

3. Bot: [After 1s]
   📚 Searching knowledge base...
   Found 23 relevant articles

4. Bot: [After 3s]
   🧠 Analyzing with GPT-4...
   Tokens used: 1,250/4,000

5. Bot: [After 5s]
   ✍️ Generating explanation...
   75% complete

6. Bot: [After 7s - Final]
   ✅ Here's a simple explanation of quantum computing:
   
   [Full response text...]
   
   ---
   📊 Stats: 7.2s | 1,847 tokens | $0.03
   
   [🔄 Regenerate] [📝 Simpler] [🎯 More Detail] [💾 Save]

7. User clicks [📝 Simpler]

8. Bot: [Updates same message]
   🔄 Making it simpler...
   
9. Bot: [After 3s]
   ✅ Here's an even simpler explanation:
   
   [Simplified response...]
```

## Security Considerations

### Permissions Required
- `app_mentions:read` - Read @mentions
- `channels:history` - Read channel messages
- `channels:read` - View channel info
- `chat:write` - Send messages
- `im:history` - Read DMs
- `im:write` - Send DMs
- `files:read` - Read uploaded files
- `reactions:read` - Read reactions
- `users:read` - Read user info

### Data Handling
- Never log sensitive message content
- Implement user-level permissions
- Respect workspace data retention policies
- Use OAuth scopes minimally
- Implement audit logging

### Error Messages
Should never expose:
- Internal API keys
- System paths
- Stack traces
- User tokens
- Internal service URLs

## Conclusion

A Slack bot integration can provide a rich, interactive interface for the AI agent with real-time progress updates, streaming responses, and interactive controls. The key is to leverage Slack's Block Kit for rich UI, message updates for progress indication, and threads for organized conversations.