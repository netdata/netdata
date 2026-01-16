# Slack Headend

Integrate agents with Slack using Socket Mode.

---

## Overview

The Slack headend allows agents to:
- Respond to direct messages
- Participate in channels
- Handle mentions and threads
- Use Block Kit for rich formatting

---

## Setup

### 1. Create Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Create New App → From scratch
3. Enable Socket Mode (Settings → Socket Mode)
4. Generate App-Level Token with `connections:write` scope

### 2. Configure Bot

OAuth & Permissions → Bot Token Scopes:
- `app_mentions:read`
- `channels:history`
- `chat:write`
- `im:history`
- `im:read`
- `im:write`

### 3. Enable Events

Event Subscriptions → Subscribe to bot events:
- `app_mention`
- `message.channels`
- `message.im`

### 4. Install to Workspace

OAuth & Permissions → Install to Workspace

---

## Configuration

In `.ai-agent.json`:

```json
{
  "slack": {
    "enabled": true,
    "appToken": "${SLACK_APP_TOKEN}",
    "botToken": "${SLACK_BOT_TOKEN}",
    "signingSecret": "${SLACK_SIGNING_SECRET}",
    "routing": {
      "rules": [
        {
          "enabled": true,
          "channels": ["#general", "#support"],
          "agent": "chat.ai",
          "engage": ["mentions", "direct-messages"]
        }
      ]
    }
  }
}
```

---

## Start Server

```bash
ai-agent --agent chat.ai --slack
```

---

## Routing Rules

### Engage Types

| Type | Description |
|------|-------------|
| `channel-posts` | Messages posted to channels |
| `direct-messages` | DMs to the bot |
| `mentions` | @mentions of the bot |
| `threads` | Replies in threads |

### Rule Configuration

```json
{
  "routing": {
    "rules": [
      {
        "enabled": true,
        "channels": ["#support"],
        "agent": "support.ai",
        "engage": ["channel-posts", "mentions"],
        "promptTemplates": {
          "channelPost": "Support request from {user.name}:\n\n{message.text}",
          "mention": "User {user.name} mentioned you:\n\n{message.text}"
        }
      },
      {
        "enabled": true,
        "channels": ["#sales"],
        "agent": "sales.ai",
        "engage": ["mentions"]
      }
    ]
  }
}
```

### Prompt Templates

Customize prompts per event type:

```json
{
  "promptTemplates": {
    "channelPost": "Channel: {channel.name}\nUser: {user.name}\n\n{message.text}",
    "directMessage": "DM from {user.name}:\n\n{message.text}",
    "mention": "Mentioned in {channel.name} by {user.name}:\n\n{message.text}",
    "threadReply": "Thread reply from {user.name}:\n\n{message.text}"
  }
}
```

---

## Available Variables

Use in prompt templates:

| Variable | Description |
|----------|-------------|
| `{user.name}` | User's display name |
| `{user.id}` | User's Slack ID |
| `{channel.name}` | Channel name |
| `{channel.id}` | Channel ID |
| `{message.text}` | Message content |
| `{message.ts}` | Message timestamp |
| `{thread.ts}` | Thread timestamp |

---

## Response Formatting

Agents can return:

### Plain Text

```
Hello! How can I help you?
```

### Markdown

Converted to Slack mrkdwn format.

### Block Kit

Return JSON with `blocks` array for rich formatting:

```json
{
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Hello!* How can I help?"
      }
    }
  ]
}
```

---

## Thread Handling

Bot responses go in threads by default:
- Channel posts → reply in thread
- Thread replies → reply in same thread
- DMs → reply directly

---

## Real-World Example

From production (Neda):

```json
{
  "slack": {
    "enabled": true,
    "appToken": "${SLACK_APP_TOKEN}",
    "botToken": "${SLACK_BOT_TOKEN}",
    "routing": {
      "rules": [
        {
          "enabled": true,
          "channels": ["#contact-us"],
          "agent": "neda.ai",
          "engage": ["channel-posts"],
          "promptTemplates": {
            "channelPost": "Contact form submission:\nChannel: {channel.name}\n\nUse company-quick-check agent first.\nDO NOT run product-messaging for this session."
          }
        },
        {
          "enabled": true,
          "channels": ["#general"],
          "agent": "neda.ai",
          "engage": ["mentions", "direct-messages"]
        }
      ]
    }
  }
}
```

---

## Troubleshooting

### Bot Not Responding

1. Check Socket Mode is enabled
2. Verify app token has `connections:write`
3. Check bot is invited to channel
4. Review event subscriptions

### Permission Errors

Add required scopes and reinstall app.

### Rate Limits

Slack rate limits apply. Use queue configuration for heavy tools.

---

## See Also

- [Headends](Headends) - Overview
- [docs/SLACK.md](../docs/SLACK.md) - Detailed Slack guide
- [docs/specs/headend-slack.md](../docs/specs/headend-slack.md) - Technical spec
