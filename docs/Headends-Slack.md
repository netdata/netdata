# Slack Headend

Deploy agents as Slack bots with Socket Mode integration, channel routing, and slash commands.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [Slack App Setup](#slack-app-setup) - Creating and configuring your Slack app
- [CLI Options](#cli-options) - Command-line configuration
- [Configuration Reference](#configuration-reference) - All config options
- [Routing Configuration](#routing-configuration) - Channel and engagement routing
- [Engagement Types](#engagement-types) - Mentions, DMs, channel posts
- [Prompt Templates](#prompt-templates) - Customizing prompts per engagement
- [Context Policy](#context-policy) - Thread context handling
- [Opener Tones](#opener-tones) - Initial response styles
- [Slash Commands](#slash-commands) - HTTP webhook commands
- [Message Flow](#message-flow) - How messages are processed
- [Progress Updates](#progress-updates) - Streaming status to Slack
- [Output Formats](#output-formats) - Markdown and Block Kit
- [Error Handling](#error-handling) - Failure modes
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The Slack headend connects your agents to Slack workspaces via Socket Mode. Use it when:
- Your team wants to interact with agents in Slack
- You need channel-specific agent routing
- You want slash command integration
- You need progress updates in threads

**Key features**:
- Socket Mode (no public endpoint required)
- Channel-based routing rules
- Per-engagement prompt templates
- Configurable opener tones
- Slack Block Kit output support
- Slash command webhooks

---

## Quick Start

### 1. Create Slack App

See [Slack App Setup](#slack-app-setup) for detailed instructions.

### 2. Configure tokens

In `.ai-agent.json`:

```json
{
  "slack": {
    "botToken": "xoxb-...",
    "appToken": "xapp-...",
    "signingSecret": "..."
  }
}
```

Or via environment variables:

```bash
export SLACK_BOT_TOKEN="xoxb-..."
export SLACK_APP_TOKEN="xapp-..."
export SLACK_SIGNING_SECRET="..."
```

### 3. Start the bot

```bash
ai-agent --agent chat.ai --slack
```

### 4. Test in Slack

- Mention your bot: `@YourBot hello`
- Send a DM to your bot
- Use slash command: `/ai-agent hello`

---

## Slack App Setup

### Step 1: Create App

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click **Create New App** → **From scratch**
3. Enter app name and select workspace
4. Click **Create App**

### Step 2: Enable Socket Mode

1. Go to **Socket Mode** in the left sidebar
2. Toggle **Enable Socket Mode** ON
3. Create an app-level token:
   - Name: `socket-mode-token`
   - Scope: `connections:write`
4. Copy the token (starts with `xapp-`)

### Step 3: Configure Bot Token Scopes

Go to **OAuth & Permissions** → **Scopes** → **Bot Token Scopes** and add:

| Scope | Purpose |
|-------|---------|
| `app_mentions:read` | Receive @mentions |
| `channels:history` | Read public channel messages |
| `channels:read` | List public channels |
| `chat:write` | Post messages |
| `groups:history` | Read private channel messages |
| `groups:read` | List private channels |
| `im:history` | Read DM messages |
| `im:read` | List DMs |
| `im:write` | Open DMs |
| `mpim:history` | Read group DM messages |
| `mpim:read` | List group DMs |
| `users:read` | Get user info |

### Step 4: Install App

1. Go to **Install App** in sidebar
2. Click **Install to Workspace**
3. Authorize the app
4. Copy the **Bot User OAuth Token** (starts with `xoxb-`)

### Step 5: Subscribe to Events

Go to **Event Subscriptions**:

1. Toggle **Enable Events** ON
2. Under **Subscribe to bot events**, add:
   - `app_mention`
   - `message.channels`
   - `message.groups`
   - `message.im`
   - `message.mpim`

### Step 6: Enable Slash Commands (Optional)

Go to **Slash Commands**:

1. Click **Create New Command**
2. Configure:
   - Command: `/ai-agent`
   - Request URL: `https://your-server/ai-agent`
   - Description: "Ask the AI agent"
3. Click **Save**

### Step 7: Get Signing Secret

Go to **Basic Information** → **App Credentials**:
- Copy **Signing Secret**

---

## CLI Options

### --slack

| Property | Value |
|----------|-------|
| Type | `boolean` |
| Default | `false` |

**Description**: Enable Slack headend. Requires tokens in config or environment.

**Example**:
```bash
ai-agent --agent chat.ai --slack
```

---

## Configuration Reference

Configuration in `.ai-agent.json` under the `slack` key:

### Authentication Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `botToken` | `string` | Yes | Bot User OAuth Token (`xoxb-...`) |
| `appToken` | `string` | Yes | App-level token for Socket Mode (`xapp-...`) |
| `signingSecret` | `string` | Yes | Signing secret for slash command verification |

**Example**:
```json
{
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "signingSecret": "${SLACK_SIGNING_SECRET}"
  }
}
```

### Engagement Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `mentions` | `boolean` | `true` | Respond to @mentions |
| `dms` | `boolean` | `true` | Respond to direct messages |
| `updateIntervalMs` | `number` or `string` | `5000` | Progress update interval |
| `historyLimit` | `number` | `10` | Max messages to include as context |
| `historyCharsCap` | `number` | `4000` | Max characters from history |
| `openerTone` | `string` | `random` | Initial message tone |

**Example**:
```json
{
  "slack": {
    "mentions": true,
    "dms": true,
    "updateIntervalMs": "3s",
    "historyLimit": 5,
    "historyCharsCap": 2000,
    "openerTone": "cheerful"
  }
}
```

### Duration Strings

`updateIntervalMs` accepts either:
- Number in milliseconds: `5000`
- Duration string: `"3s"`, `"500ms"`, `"1m"`

---

## Routing Configuration

Route messages to different agents based on channels and engagement types.

### Basic Structure

```json
{
  "slack": {
    "routing": {
      "default": { ... },
      "rules": [ ... ],
      "deny": [ ... ]
    }
  }
}
```

### Default Route

The fallback when no rules match:

```json
{
  "routing": {
    "default": {
      "agent": "./agents/general.ai",
      "engage": ["mentions", "dms"],
      "promptTemplates": {
        "mention": "User @{user} mentioned you in #{channel}: {message}",
        "dm": "User @{user} sent you a DM: {message}"
      }
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `agent` | `string` | Path to agent file (relative to primary agent) |
| `engage` | `array` | Engagement types to handle: `mentions`, `dms`, `channel-posts` |
| `promptTemplates` | `object` | Custom prompts per engagement type |
| `contextPolicy` | `object` | Thread context handling |

### Routing Rules

Ordered rules evaluated before the default:

```json
{
  "routing": {
    "rules": [
      {
        "channels": ["#support", "#help-*"],
        "agent": "./agents/support.ai",
        "engage": ["mentions", "channel-posts"],
        "enabled": true
      },
      {
        "channels": ["#engineering", "C012ABCDE"],
        "agent": "./agents/technical.ai",
        "engage": ["mentions"]
      }
    ]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `channels` | `array` | Channel patterns (names or IDs) |
| `agent` | `string` | Path to agent file |
| `engage` | `array` | Engagement types |
| `enabled` | `boolean` | Enable/disable rule (default: `true`) |
| `promptTemplates` | `object` | Custom prompts |
| `contextPolicy` | `object` | Context handling |

### Channel Patterns

Channels can be specified as:
- **Name**: `#general`, `support` (with or without `#`)
- **ID**: `C012ABCDE`, `G012ABCDE`
- **Glob pattern**: `#help-*`, `#team-*-support`

### Deny Rules

Block specific channels or engagement types:

```json
{
  "routing": {
    "deny": [
      {
        "channels": ["#announcements", "#readonly-*"],
        "engage": ["mentions", "channel-posts", "dms"]
      },
      {
        "channels": ["#general"],
        "engage": ["channel-posts"]
      }
    ]
  }
}
```

Deny rules are evaluated **first**. If a deny matches, the message is ignored.

---

## Engagement Types

| Type | Trigger | Description |
|------|---------|-------------|
| `mentions` | `@YourBot hello` | User mentions the bot in a channel |
| `dms` | Direct message | User sends a direct message to the bot |
| `channel-posts` | Any message | Any message in a channel (use carefully) |

**Warning**: `channel-posts` responds to ALL messages in matched channels. Use with caution and deny rules.

---

## Prompt Templates

Customize how user messages are presented to agents:

```json
{
  "promptTemplates": {
    "mention": "User @{user} in #{channel} asks: {message}",
    "dm": "Direct message from @{user}: {message}",
    "channelPost": "Message in #{channel} from @{user}: {message}"
  }
}
```

### Available Variables

| Variable | Description |
|----------|-------------|
| `{user}` | User's display name |
| `{userId}` | User's Slack ID |
| `{channel}` | Channel name |
| `{channelId}` | Channel ID |
| `{message}` | The user's message text |
| `{thread}` | Thread context (if in thread) |

---

## Context Policy

Control how thread context is included:

```json
{
  "contextPolicy": {
    "channelPost": "selfAndPrevious"
  }
}
```

| Value | Description |
|-------|-------------|
| `selfOnly` | Only include the current message |
| `previousOnly` | Include previous messages in thread, not current |
| `selfAndPrevious` | Include both (default) |

---

## Opener Tones

Configure the initial response message style:

```json
{
  "slack": {
    "openerTone": "cheerful"
  }
}
```

| Tone | Example |
|------|---------|
| `random` | Randomly selects from the others |
| `cheerful` | "On it! Let me help you with that..." |
| `formal` | "I'm processing your request..." |
| `busy` | "Working on it..." |

---

## Slash Commands

### Configuration

Slash commands require an HTTP endpoint. Use with REST headend:

```bash
ai-agent --agent chat.ai --slack --api 8080
```

Or the Slack headend creates a fallback HTTP server if no REST headend is present.

### Slash Command Endpoint

The endpoint is registered at `/ai-agent` (configurable):

```json
{
  "api": {
    "port": 8080
  }
}
```

### Request Flow

1. User types `/ai-agent what is the weather?`
2. Slack sends POST to your endpoint
3. ai-agent verifies signature using `signingSecret`
4. Agent processes the prompt
5. Response posted back to Slack

### Signature Verification

Requests are verified using the Slack signing secret:
- `X-Slack-Signature` header
- `X-Slack-Request-Timestamp` header

---

## Message Flow

### 1. Event Received

Socket Mode receives message event from Slack.

### 2. Route Resolution

1. Check deny rules (if match, ignore message)
2. Check routing rules in order (first match wins)
3. Fall back to default route
4. If no route, ignore message

### 3. Concurrency Check

Acquire session slot (queue if limit reached).

### 4. Post Opener

Send initial "thinking" message based on `openerTone`.

### 5. Run Agent

Execute agent session with:
- User prompt (from template)
- Thread context (if configured)
- Output format: `slack-block-kit` or `markdown`

### 6. Stream Progress

Post progress updates to thread at `updateIntervalMs` intervals.

### 7. Post Final Response

Update opener message with final agent response.

### 8. Release Slot

Return concurrency slot for next request.

---

## Progress Updates

During agent execution, progress is streamed to the thread:

```
[12:34:56] Starting research...
[12:34:58] Querying database...
[12:35:02] Analyzing results...
```

Configure update frequency:

```json
{
  "slack": {
    "updateIntervalMs": 3000
  }
}
```

---

## Output Formats

### Markdown

Default format. Converted to Slack mrkdwn:

```markdown
# Hello

This is **bold** and _italic_.

- Item 1
- Item 2
```

### Slack Block Kit

For rich formatting, agents can output Block Kit JSON:

```yaml
---
output:
  format: slack-block-kit
---
```

The agent must return valid [Slack Block Kit](https://api.slack.com/block-kit) structures.

---

## Error Handling

### Connection Errors

Socket Mode automatically reconnects on connection failures.

### Agent Failures

If an agent fails:
1. Error logged with details
2. Error message posted to thread
3. Concurrency slot released

### Rate Limiting

Slack has rate limits. The headend:
- Respects `Retry-After` headers
- Queues messages during rate limiting
- Logs rate limit events

---

## Troubleshooting

### Bot not responding to mentions

**Symptom**: Bot doesn't respond when mentioned.

**Possible causes**:
1. `mentions` set to `false`
2. Deny rule blocking the channel
3. Bot not in the channel
4. Event subscription missing

**Solutions**:
1. Check `"mentions": true` in config
2. Review deny rules
3. Invite bot to channel: `/invite @YourBot`
4. Verify `app_mention` event subscription

### Bot not responding to DMs

**Symptom**: No response in direct messages.

**Possible causes**:
1. `dms` set to `false`
2. Missing `im:*` scopes
3. Event subscription missing

**Solutions**:
1. Check `"dms": true` in config
2. Verify OAuth scopes include `im:history`, `im:read`, `im:write`
3. Verify `message.im` event subscription

### "Invalid signature" on slash commands

**Symptom**: Slash commands fail with signature error.

**Cause**: Signing secret mismatch.

**Solution**:
1. Copy signing secret from **Basic Information** → **App Credentials**
2. Update `signingSecret` in config
3. Restart ai-agent

### Socket Mode disconnections

**Symptom**: Bot goes offline frequently.

**Possible causes**:
1. Network instability
2. Invalid app token
3. Token expired

**Solutions**:
1. Check network connectivity
2. Verify `appToken` starts with `xapp-`
3. Regenerate app-level token in Slack app settings

### Messages in wrong channels triggering bot

**Symptom**: Bot responds in unintended channels.

**Cause**: `channel-posts` enabled without proper routing.

**Solution**: Add deny rules for channels that shouldn't trigger:
```json
{
  "routing": {
    "deny": [
      { "channels": ["#general"], "engage": ["channel-posts"] }
    ]
  }
}
```

### Progress updates too frequent/slow

**Symptom**: Updates flood the thread or are too slow.

**Solution**: Adjust `updateIntervalMs`:
```json
{
  "slack": {
    "updateIntervalMs": "5s"
  }
}
```

### Bot token invalid

**Symptom**: `invalid_auth` or `not_authed` errors.

**Possible causes**:
1. Token incorrect
2. Token revoked
3. App not installed to workspace

**Solutions**:
1. Verify token starts with `xoxb-`
2. Check app is installed in Slack app settings
3. Reinstall app and copy new token

---

## Complete Configuration Example

```json
{
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "signingSecret": "${SLACK_SIGNING_SECRET}",
    "mentions": true,
    "dms": true,
    "updateIntervalMs": "3s",
    "historyLimit": 10,
    "historyCharsCap": 4000,
    "openerTone": "cheerful",
    "routing": {
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"]
      },
      "rules": [
        {
          "channels": ["#support", "#help-*"],
          "agent": "./agents/support.ai",
          "engage": ["mentions", "channel-posts"],
          "promptTemplates": {
            "mention": "Support request from @{user}: {message}",
            "channelPost": "Support question in #{channel}: {message}"
          }
        },
        {
          "channels": ["#engineering"],
          "agent": "./agents/technical.ai",
          "engage": ["mentions"],
          "contextPolicy": {
            "channelPost": "selfOnly"
          }
        }
      ],
      "deny": [
        {
          "channels": ["#announcements", "#readonly-*"],
          "engage": ["mentions", "channel-posts", "dms"]
        }
      ]
    }
  },
  "api": {
    "port": 8080
  }
}
```

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Configuration](Configuration) - General configuration
- [Agent Files](Agent-Files) - Agent configuration
- [specs/headend-slack.md](specs/headend-slack.md) - Technical specification
