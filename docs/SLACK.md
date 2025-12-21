# Slack Integration Guide

This guide covers everything you need to know about integrating ai-agent with Slack, from initial setup to advanced configuration.

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Quick Start](#quick-start)
4. [Configuration](#configuration)
5. [Running the Integration](#running-the-integration)
6. [Interaction Modes](#interaction-modes)
7. [Routing & Channel Personas](#routing--channel-personas)
8. [Advanced Features](#advanced-features)
9. [Troubleshooting](#troubleshooting)
10. [Reference](#reference)

## Overview

The ai-agent Slack integration allows your AI agents to interact directly with your Slack workspace through multiple interaction modes:

- **Reactive Bot**: Responds to @mentions, direct messages, and optionally all channel posts
- **Slash Commands**: Invokes agents via slash commands (e.g., `/ai <prompt>`)
- **Message Shortcuts**: Right-click context menu action on any message

All modes support:
- Real-time progress updates with Block Kit UI
- Multi-agent orchestration
- Per-channel routing and personas
- Conversation context awareness
- Interactive Stop/Abort controls

### Connection Mode

The integration runs in **Socket Mode** by default, which means:
- No public endpoint required for mentions, DMs, or message shortcuts
- Secure WebSocket connection initiated from your server to Slack
- Perfect for development and internal deployments
- Slash commands optionally require a public HTTPS endpoint

## Prerequisites

Before you start, you'll need:

1. **Slack Workspace**: Admin access to create apps
2. **AI Agent Installation**: Working ai-agent installation
3. **Agent File**: At least one `.ai` agent file configured
4. **Internet Connection**: For Socket Mode connectivity

## Quick Start

### Step 1: Create Your Slack App

1. Go to https://api.slack.com/apps
2. Click **"Create New App"** → **"From an app manifest"**
3. Select your workspace
4. Use this manifest template (customize the names to fit your needs):

```json
{
  "display_information": {
    "name": "Your Bot Name",
    "description": "Your bot description",
    "background_color": "#2b6cb0"
  },
  "features": {
    "bot_user": {
      "display_name": "your-bot",
      "always_online": true
    },
    "shortcuts": [
      {
        "name": "Ask Your Bot",
        "type": "message",
        "callback_id": "ask_bot",
        "description": "Ask your bot about this message"
      }
    ],
    "slash_commands": [
      {
        "command": "/your-command",
        "url": "https://YOUR_PUBLIC_URL/slack/commands",
        "description": "Invoke your bot",
        "usage_hint": "<prompt>",
        "should_escape": false
      }
    ]
  },
  "oauth_config": {
    "scopes": {
      "bot": [
        "app_mentions:read",
        "chat:write",
        "channels:read",
        "channels:history",
        "im:history",
        "im:write",
        "mpim:history",
        "groups:history",
        "users:read",
        "users:read.email",
        "commands"
      ]
    }
  },
  "settings": {
    "event_subscriptions": {
      "bot_events": [
        "app_mention",
        "message.im",
        "message.channels",
        "message.groups"
      ]
    },
    "interactivity": {
      "is_enabled": true
    },
    "socket_mode_enabled": true
  }
}
```

5. **Customize the manifest:**
   - **MUST keep:** `"callback_id": "ask_bot"` (hardcoded in ai-agent)
   - **Customize freely:** Bot name, slash command name, descriptions, display names
   - **Update if using slash commands:** Replace `YOUR_PUBLIC_URL` with your actual endpoint

6. Review and create the app

### Step 2: Get Your Tokens

After creating the app:

1. Go to **"OAuth & Permissions"**
   - Click **"Install to Workspace"**
   - Copy the **Bot User OAuth Token** (starts with `xoxb-`)

2. Go to **"Basic Information"**
   - Under "App-Level Tokens", click **"Generate Token and Scopes"**
   - Add the `connections:write` scope
   - Copy the **App-Level Token** (starts with `xapp-`)

3. Still in **"Basic Information"**
   - Find **"Signing Secret"** under "App Credentials"
   - Copy it (only needed for slash commands)

### Step 3: Configure ai-agent

Create or update `.ai-agent.json` in your working directory:

```json
{
  "slack": {
    "enabled": true,
    "mentions": true,
    "dms": true,
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "signingSecret": "${SLACK_SIGNING_SECRET}"
  }
}
```

Create `.ai-agent.env` (DO NOT commit this file):

```bash
SLACK_BOT_TOKEN=xoxb-your-token-here
SLACK_APP_TOKEN=xapp-your-token-here
SLACK_SIGNING_SECRET=your-signing-secret-here
```

### Step 4: Invite the Bot

In your Slack workspace:
- Go to any channel where you want the bot
- Type `/invite @your-bot` (use the display name you chose)
- The bot is now ready to respond to mentions in that channel

### Step 5: Run ai-agent

Start the agent with Slack enabled:

```bash
ai-agent \
  --agent ./your-agent.ai \
  --slack \
  --verbose
```

### Step 6: Test It

Try these interactions:

1. **Mention**: In a channel, type `@your-bot hello`
2. **Direct Message**: Send a DM to the bot
3. **Message Shortcut**: Right-click any message → "More actions" → Your shortcut name
4. **Slash Command**: Type `/your-command what is the weather?` (requires public endpoint)

## Configuration

### Basic Options

All options go under the `slack` section in `.ai-agent.json`:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable Slack integration |
| `mentions` | boolean | `true` | Respond to @mentions |
| `dms` | boolean | `true` | Respond to direct messages |
| `botToken` | string | - | Bot OAuth token (xoxb-...) |
| `appToken` | string | - | App-level token (xapp-...) |
| `signingSecret` | string | - | Signing secret (for slash commands) |
| `updateIntervalMs` | number | `2000` | Progress update frequency (ms) |
| `historyLimit` | number | `100` | Max messages to fetch for context |
| `historyCharsCap` | number | `100000` | Max characters in context |
| `openerTone` | string | `"random"` | Opener message tone (see below) |

### Opener Tones

Control the personality of initial acknowledgment messages:

- `"random"`: Mix of all styles (default)
- `"cheerful"`: Upbeat and energetic ("Awesome, let's go!")
- `"formal"`: Professional and polite ("Acknowledged, proceeding...")
- `"busy"`: Quick and efficient ("On it now...")

Example:

```json
{
  "slack": {
    "openerTone": "formal"
  }
}
```

### API Port Configuration

For slash commands, specify the API port:

```json
{
  "api": {
    "port": 8080
  },
  "slack": {
    "signingSecret": "${SLACK_SIGNING_SECRET}"
  }
}
```

## Running the Integration

### Basic Launch

Minimal command for Socket Mode (mentions, DMs, shortcuts):

```bash
ai-agent --agent ./agent.ai --slack
```

### With API for Slash Commands

If you want slash commands:

```bash
ai-agent \
  --agent ./agent.ai \
  --slack \
  --api 8080
```

### With Verbose Logging

See detailed Slack event logs:

```bash
ai-agent \
  --agent ./agent.ai \
  --slack \
  --verbose
```

### Multiple Agents

Route different channels to different agents:

```bash
ai-agent \
  --agent ./agents/support.ai \
  --agent ./agents/research.ai \
  --agent ./agents/general.ai \
  --slack
```

Configure routing in `.ai-agent.json` (see [Routing](#routing--channel-personas) section).

### Production Deployment

For production, run as a systemd service:

1. Create `/etc/systemd/system/ai-agent-slack.service`:

```ini
[Unit]
Description=AI Agent Slack Bot
After=network.target

[Service]
Type=simple
User=ai-agent
WorkingDirectory=/opt/ai-agent
ExecStart=/opt/ai-agent/bin/ai-agent \
  --agent /opt/ai-agent/agents/main.ai \
  --slack \
  --api 8080
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

2. Enable and start:

```bash
sudo systemctl enable ai-agent-slack
sudo systemctl start ai-agent-slack
sudo journalctl -u ai-agent-slack -f
```

## Interaction Modes

### 1. Mentions

When someone mentions your bot in a channel:

```
User: @ai-agent what's the status of project X?
Bot: [progress updates appear]
Bot: [final response with Block Kit formatting]
```

**Features:**
- Responds only when explicitly mentioned
- Includes recent channel context (configurable)
- Posts progress updates in real-time
- Shows Stop/Abort buttons during execution

**Requirements:**
- Bot must be invited to the channel (`/invite @ai-agent`)
- `mentions: true` in config
- `app_mentions:read` scope

### 2. Direct Messages

Private conversations with the bot:

```
User: (DM to bot) analyze the customer data
Bot: [personalized opener]
Bot: [progress updates]
Bot: [final response]
```

**Features:**
- One-on-one private interaction
- Full conversation context
- Personal opener messages
- Same progress indicators as mentions

**Requirements:**
- `dms: true` in config
- `im:history`, `im:write` scopes

### 3. Channel Posts (Auto-Response)

Automatically respond to all messages in specific channels:

**Features:**
- No @mention required
- Per-channel routing rules
- Useful for support/monitoring channels
- Self-only context (no history) by default

**Requirements:**
- Explicit routing rule with `"engage": ["channel-posts"]`
- `message.channels`, `message.groups` bot events
- `channels:history`, `groups:history` scopes

**Important**: The bot will NOT auto-respond unless you configure routing rules. See [Routing](#routing--channel-personas).

### 4. Message Shortcuts

Right-click context menu on any message:

1. Right-click (or click "⋮") on any Slack message
2. Select "More actions" → Your shortcut name
3. Bot analyzes the message and responds

**Features:**
- Works on any message, even in channels where bot isn't a member
- If bot isn't a channel member, continues in DM
- Uses channel-post routing rules
- Includes message permalink in context

**Requirements:**
- Message shortcut defined in manifest (`callback_id: "ask_bot"`)
- `interactivity.is_enabled: true`
- Works entirely over Socket Mode (no public endpoint)

### 5. Slash Commands

Invoke the bot via command:

```
/your-command analyze this log file
```

**Features:**
- Works in any channel or DM
- Uses channel-post routing rules
- Public or ephemeral responses

**Requirements:**
- Public HTTPS endpoint at `https://your-domain/slack/commands`
- Slash command defined in manifest
- `signingSecret` in config
- `commands` scope

**Setup for Development:**

Use a tunnel to expose your local endpoint:

```bash
# Using cloudflared
cloudflared tunnel --url http://localhost:8080

# Or using ngrok
ngrok http 8080
```

Then update your slash command URL in the Slack app manifest to point to your tunnel.

## Routing & Channel Personas

Routing allows you to:
- Use different agents for different channels
- Auto-respond to channel posts in specific channels
- Customize prompts per channel
- Block responses in certain channels

### Basic Routing Structure

```json
{
  "slack": {
    "routing": {
      "deny": [
        { "channels": ["#customer-*"], "engage": ["channel-posts"] }
      ],
      "rules": [
        {
          "channels": ["#support", "#sla"],
          "agent": "./agents/support.ai",
          "engage": ["channel-posts", "mentions"],
          "promptTemplates": {
            "channelPost": "You are a support engineer in {channel.name}. Be concise."
          }
        }
      ],
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"]
      }
    }
  }
}
```

### Routing Precedence

When an event arrives, routing is resolved in this order:

1. **Deny rules** - Check all deny rules first
2. **First matching rule** - First rule in `rules[]` that matches
3. **Default** - Falls back to default if `engage` includes the event type

### Channel Matching

Channels can be specified as:
- **Names**: `#support`, `support` (case-insensitive, `#` optional)
- **IDs**: `C1234567890`, `G9876543210`
- **Wildcards**: `#support-*`, `#team-?-channel`

Examples:
- `"#support"` - Exact channel name
- `"#support-*"` - All channels starting with "support-"
- `"C1234567890"` - Specific channel ID
- `"G*"` - All private channels (groups)

### Engage Types

Controls which interaction types trigger the agent:

- `"mentions"` - @mentions in channels
- `"dms"` - Direct messages
- `"channel-posts"` - All channel messages (not just mentions)

### Prompt Templates

Customize the prompt sent to the agent for each interaction type:

```json
{
  "promptTemplates": {
    "mention": "User {user.label} mentioned you in {channel.name}: {text}",
    "dm": "Private message from {user.label}: {text}",
    "channelPost": "Channel post in {channel.name} by {user.label}: {text}"
  }
}
```

**Available Variables:**
- `{text}` - User's message text
- `{user.id}` - Slack user ID (U123...)
- `{user.label}` - User's name (real name, display name, or email)
- `{channel.id}` - Channel ID (C123... or G123...)
- `{channel.name}` - Channel name (e.g., "support")
- `{ts}` - Timestamp (human-readable)
- `{message.url}` - Permalink to the message

### Context Policies

Control how much conversation context is included:

```json
{
  "contextPolicy": {
    "channelPost": "selfOnly"
  }
}
```

**Options for `channelPost`:**
- `"selfOnly"` (default) - Only the current message
- `"previousOnly"` - Only previous messages, not current
- `"selfAndPrevious"` - Both current and previous messages

**Note**: Mentions and DMs always fetch recent context (controlled by `historyLimit`).

### Routing Examples

#### Example 1: Support Channels with Auto-Response

```json
{
  "slack": {
    "routing": {
      "rules": [
        {
          "channels": ["#support", "#sla", "#help-*"],
          "agent": "./agents/support.ai",
          "engage": ["channel-posts", "mentions"],
          "promptTemplates": {
            "channelPost": "Support ticket in {channel.name}. User {user.label}: {text}"
          }
        }
      ],
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"]
      }
    }
  }
}
```

#### Example 2: Department-Specific Agents

```json
{
  "slack": {
    "routing": {
      "rules": [
        {
          "channels": ["#eng-*"],
          "agent": "./agents/engineering.ai",
          "engage": ["mentions"]
        },
        {
          "channels": ["#sales-*"],
          "agent": "./agents/sales.ai",
          "engage": ["mentions"]
        },
        {
          "channels": ["#marketing-*"],
          "agent": "./agents/marketing.ai",
          "engage": ["mentions"]
        }
      ],
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"]
      }
    }
  }
}
```

#### Example 3: Deny with Wildcards

Prevent bot from responding in customer-facing channels:

```json
{
  "slack": {
    "routing": {
      "deny": [
        {
          "channels": ["#customer-*", "#client-*", "#external-*"],
          "engage": ["channel-posts", "mentions", "dms"]
        }
      ],
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"]
      }
    }
  }
}
```

## Advanced Features

### Real-Time Progress Updates

The bot automatically shows live progress during agent execution:

```
🔍 Starting analysis…

┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ 🤖 support-agent (5.2s)                     ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

  🔄 searching_knowledge_base — searching for 'API errors'
  🔄 analyze_logs — analyzing last 100 lines

📊 12.3k in • 4.5k out • 2 tools • 1 agent

[Stop] [Abort]
```

**Features:**
- Updates every 2 seconds (configurable via `updateIntervalMs`)
- Shows active tools and sub-agents
- Progress header uses `task_status.now` (clamped to 150 chars); `status/done/pending` render as detail lines when present
- Token usage tracking
- Interactive Stop/Abort buttons

### Stop vs Abort

During execution, users can control the agent:

- **Stop** (graceful): Allows current tool to finish, then stops
- **Abort** (immediate): Cancels immediately, interrupting current tool

The buttons disappear once execution completes.

### Multi-Agent Orchestration

When an agent calls sub-agents, progress shows the full hierarchy:

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ 🤖 master-agent (15.2s)                     ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
  ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
  ┃ 🤖 research-agent (8.3s)                  ┃
  ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
    🔄 web_search — searching 'competitor analysis'

  🔄 analyze_data — processing results

📊 18.7k in • 6.2k out • 5 tools • 2 agents
```

### Block Kit Responses

Agents can output rich Slack Block Kit messages:

**In your agent's output format:**

```yaml
---
description: Support Agent
output:
  format: json
  schema:
    type: object
    properties:
      messages:
        type: array
        items:
          type: object
          properties:
            blocks:
              type: array
---
```

**Agent returns:**

```json
{
  "messages": [
    {
      "blocks": [
        {
          "type": "header",
          "text": {
            "type": "plain_text",
            "text": "🎯 Support Summary"
          }
        },
        {
          "type": "section",
          "text": {
            "type": "mrkdwn",
            "text": "*Status:* All systems operational"
          }
        },
        {
          "type": "divider"
        }
      ]
    }
  ]
}
```

The bot will render these as native Slack blocks automatically.

### Slack mrkdwn rules (NOT GitHub Markdown)

When emitting `slack-block-kit`, the text inside `section` and `context` blocks must be Slack *mrkdwn*:

- Do **not** use Markdown headings (`#`, `##`, `###`) or Markdown tables (`|---|`) — Slack will show them literally.
- Use header blocks for titles. For subheadings inside a section, use a bold line (e.g., `*Section Title*`) and a newline.
- Allowed formatting: `*bold*`, `_italic_`, `~strikethrough~`, `inline code`, fenced code blocks (```code```), bullets (`•` or `-`).
- Links must use Slack format: `<https://example.com|link text>` — do not use `[text](url)`.
- Escape special characters in text: `&` → `&amp;`, `<` → `&lt;`, `>` → `&gt;`.
- Tables are **not** allowed in mrkdwn. For 2-column layouts, use Block Kit `section.fields` (max 10 fields) instead of Markdown tables.
- Avoid HTML, Mermaid fences, or raw JSON inside text blocks.

### Concurrency Control

The Slack headend has built-in concurrency limiting:

- Default: 10 concurrent agent runs
- Prevents resource exhaustion
- Queues additional requests
- Per-run timeout controls

### User Information

The bot automatically resolves user labels including:
- Real name
- Display name
- Email address
- User ID

Cached for performance, so repeated mentions are fast.

## Troubleshooting

### Bot Doesn't Respond to Mentions

**Check:**
1. Is the bot invited to the channel? Type `/invite @ai-agent`
2. Is `mentions: true` in config?
3. Check logs for `[IGNORED]` messages with `--verbose`
4. Verify `app_mentions:read` scope in Slack app
5. Ensure Socket Mode is enabled in Slack app settings

### DMs Not Working

**Check:**
1. Is `dms: true` in config?
2. Are `im:history` and `im:write` scopes enabled?
3. Try sending a DM to see error logs

### Channel Posts Not Auto-Responding

**This is by design!** Auto-response requires explicit routing:

1. Add a routing rule with `"engage": ["channel-posts"]`
2. Specify channels in the rule
3. Ensure `message.channels` event is in manifest
4. Bot must be a member of the channel

Example:

```json
{
  "slack": {
    "routing": {
      "rules": [
        {
          "channels": ["#support"],
          "agent": "./agent.ai",
          "engage": ["channel-posts"]
        }
      ]
    }
  }
}
```

### Slash Command Returns "invalid signature"

**Check:**
1. Is `signingSecret` correct in config?
2. Is system time accurate? (Signature window is ±5 minutes)
3. Is the Request URL correct in Slack app settings?
4. Are you using the correct endpoint `/slack/commands`?

**Test signature locally:**

```bash
export SLACK_SIGNING_SECRET=your-secret
data='token=fake&team_id=T123&channel_id=C123&user_id=U123&command=/your-command&text=hello'
ts=$(date +%s)
base="v0:$ts:$data"
sig="v0=$(echo -n "$base" | openssl dgst -sha256 -hmac "$SLACK_SIGNING_SECRET" -hex | sed 's/^.* //')"
curl -i -X POST http://localhost:8080/slack/commands \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "X-Slack-Request-Timestamp: $ts" \
  -H "X-Slack-Signature: $sig" \
  --data "$data"
```

### Message Shortcut Not Appearing

**Check:**
1. Is the shortcut in the manifest?
2. Is `callback_id: "ask_bot"` correct?
3. Is `interactivity.is_enabled: true`?
4. Did you reinstall the app after manifest changes?

To fix:
1. Go to Slack App settings → "App Manifest"
2. Verify shortcut exists
3. Save changes
4. Go to "Install App" → "Reinstall to Workspace"

### Routing Not Working

**Check:**
1. Review channel names/IDs (names are case-insensitive)
2. Test with `--verbose` to see routing resolution logs
3. Remember: deny rules checked first, then rules, then default
4. Verify `engage` includes the interaction type

**Debug with logs:**

```bash
ai-agent --agent ./agent.ai --slack --verbose 2>&1 | grep -i routing
```

### Bot Not Updating Progress

**Check:**
1. Are you seeing any initial message? If not, check basic setup
2. Try increasing `updateIntervalMs` (default is 2000ms)
3. Check for Slack API rate limits in logs
4. Verify agent is actually making progress (check with `--verbose`)

### Response Too Large / Formatting Errors

The bot handles Slack's limits automatically, but if you see errors:

**Solutions:**
1. Have your agent output Block Kit format (see [Block Kit Responses](#block-kit-responses))
2. Reduce response length in agent's instructions
3. Split large responses into multiple messages
4. Check for invalid Block Kit structure in agent output

**Slack Limits:**
- Text: 40,000 characters max
- Blocks: 50 blocks max per message
- Byte limit: varies by block type

### Connection Issues

**Socket Mode disconnects:**
- Normal! Socket Mode automatically reconnects
- Check internet connectivity
- Verify app token (xapp-) is valid
- Look for `socket_mode_enabled: true` in manifest

**To monitor:**

```bash
ai-agent --agent ./agent.ai --slack --trace-slack --verbose
```

### Performance Issues

**If bot is slow:**
1. Reduce `historyLimit` (default 100)
2. Reduce `historyCharsCap` (default 100k)
3. Use `"contextPolicy": { "channelPost": "selfOnly" }` for channel posts
4. Check agent's tool execution times
5. Consider using faster LLM models

**If many concurrent requests:**
- Default concurrency limit is 10
- Requests queue automatically
- Consider running multiple instances with separate agents

## Reference

### Complete Configuration Example

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "type": "openai"
    }
  },
  "api": {
    "enabled": true,
    "port": 8080
  },
  "slack": {
    "enabled": true,
    "mentions": true,
    "dms": true,
    "updateIntervalMs": 2000,
    "historyLimit": 100,
    "historyCharsCap": 100000,
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "signingSecret": "${SLACK_SIGNING_SECRET}",
    "openerTone": "random",
    "routing": {
      "deny": [
        {
          "channels": ["#customer-*", "#external-*"],
          "engage": ["channel-posts"]
        }
      ],
      "rules": [
        {
          "channels": ["#support", "#sla"],
          "agent": "./agents/support.ai",
          "engage": ["channel-posts", "mentions"],
          "promptTemplates": {
            "channelPost": "Support channel {channel.name}. User {user.label}: {text}",
            "mention": "Support request from {user.label} in {channel.name}: {text}"
          },
          "contextPolicy": {
            "channelPost": "selfOnly"
          }
        },
        {
          "channels": ["#eng-*"],
          "agent": "./agents/engineering.ai",
          "engage": ["mentions"]
        }
      ],
      "default": {
        "agent": "./agents/general.ai",
        "engage": ["mentions", "dms"],
        "promptTemplates": {
          "mention": "Question from {user.label}: {text}",
          "dm": "Private message from {user.label}: {text}"
        }
      }
    }
  }
}
```

### Required Slack App Scopes

**Bot Token Scopes** (xoxb-):
- `app_mentions:read` - Read @mentions
- `chat:write` - Send messages
- `channels:read` - View channel info
- `channels:history` - Read channel messages
- `im:history` - Read DMs
- `im:write` - Send DMs
- `mpim:history` - Read multi-person DMs
- `groups:history` - Read private channels
- `users:read` - Read user info
- `users:read.email` - Read user emails
- `commands` - Slash commands

**App-Level Token Scopes** (xapp-):
- `connections:write` - Socket Mode connectivity

### Required Slack Bot Events

For Socket Mode event subscriptions:
- `app_mention` - @mentions
- `message.im` - DMs
- `message.channels` - Channel posts (for auto-response)
- `message.groups` - Private channel posts

### Template Variables Reference

Use these in `promptTemplates`:

| Variable | Example | Description |
|----------|---------|-------------|
| `{text}` | `"what's the status?"` | User's message text |
| `{user.id}` | `U1234567890` | Slack user ID |
| `{user.label}` | `John Doe (john.doe@company.com, U123...)` | Full user label |
| `{channel.id}` | `C1234567890` | Slack channel ID |
| `{channel.name}` | `support` | Channel name (without #) |
| `{ts}` | `12/1/2024, 2:30:15 PM` | Human-readable timestamp |
| `{message.url}` | `https://workspace.slack.com/archives/C.../p...` | Message permalink |

### API Endpoints

When running with `--api`:

- `GET /health` - Health check
- `POST /slack/commands` - Slash command handler (requires public endpoint)

### Environment Variables

For security, use `.ai-agent.env`:

```bash
# Required for Socket Mode
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...

# Optional: for slash commands
SLACK_SIGNING_SECRET=...

# Optional: LLM provider keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

**Important:** Add `*.env` to `.gitignore` to prevent committing secrets!

### Command-Line Options

Slack-specific options:

```bash
ai-agent \
  --agent PATH              # Agent file (required, repeatable)
  --slack                   # Enable Slack integration
  --api PORT                # Enable API on port (for slash commands)
  --verbose                 # Detailed logging
  --trace-slack             # Trace Slack SDK calls
  --trace-llm               # Trace LLM requests
  --trace-mcp               # Trace MCP tool calls
```

### Logging

With `--verbose`, you'll see detailed logs:

```
[SLACK→AGENT] runId=abc123 kind=mention channel="support"/C123...
              user="John Doe"/U123... text="what's the status?"
              context=included agent=default

[AGENT→SLACK] runId=abc123 channel="support"/C123...
              user="John Doe" response=blocks(3) error="none"
```

These logs show:
- Interaction type (mention, dm, channel-post)
- Channel and user info
- Context inclusion
- Response format
- Any errors

### Related Documentation

- **[Main README](../README.md)**: Overview and quick start
- **[SPECS.md](SPECS.md)**: Technical specifications
- **[MULTI-AGENT.md](MULTI-AGENT.md)**: Multi-agent orchestration
- **[TESTING.md](TESTING.md)**: Testing framework

## Getting Help

If you encounter issues:

1. Check [Troubleshooting](#troubleshooting) section above
2. Run with `--verbose` to see detailed logs
3. Check Slack app settings and scopes
4. Review the configuration examples
5. Open an issue: https://github.com/netdata/ai-agent/issues

## Security Best Practices

1. **Never commit tokens**: Use `.ai-agent.env` and add to `.gitignore`
2. **Rotate tokens regularly**: Generate new tokens periodically
3. **Limit scopes**: Only enable required OAuth scopes
4. **Use deny rules**: Block sensitive channels explicitly
5. **Monitor usage**: Track agent activity with `--verbose`
6. **Review responses**: Ensure agents don't leak sensitive data
7. **Secure endpoints**: For slash commands, use HTTPS and verify signatures

## Summary

The ai-agent Slack integration provides:

- **Zero-config Socket Mode**: No public endpoints needed for most features
- **Multiple interaction modes**: Mentions, DMs, channel posts, shortcuts, slash commands
- **Intelligent routing**: Per-channel agents and personas
- **Real-time progress**: Live updates with Block Kit UI
- **Production-ready**: Concurrency controls, error handling, automatic retries
- **Secure by default**: Token-based auth, signature verification, deny rules

Start with the [Quick Start](#quick-start) to get up and running in minutes!
