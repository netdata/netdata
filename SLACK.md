# Slack Integration Guide

This document explains all Slack features of ai-agent, how they work, what configuration is needed, and how to apply Slack App manifest changes. It covers three feature sets:

- Reacting Bot (mentions, DMs, and channel posts)
- Slash Commands (public endpoint)
- Context Menu (message shortcut: "Ask Neda")

All features are compatible with Socket Mode. Slash commands additionally require a publicly reachable HTTPS endpoint.

-------------------------------------------------------------------------------

## Reacting Bot

Summary
- What it does:
  - Responds to `@app` mentions in channels and to DMs with the bot.
  - Optionally auto-responds to channel posts (non-mentions) per channel via routing rules.
  - Supports per-channel personas, wildcards, deny-lists, and per-kind prompt templates.
  - Tracks live run progress in a thread with Stop/Abort controls. Posts result as Block Kit or text.
- Required config:
  - Slack tokens: `SLACK_BOT_TOKEN` (xoxb), `SLACK_APP_TOKEN` (xapp)
  - `.ai-agent.json` → `slack` section with `enabled`, `mentions`, `dms`, and optional `routing` rules
- Required scopes/manifest:
  - Bot events: `app_mention`, `message.im`, and (for channel posts) `message.channels`, `message.groups`
  - Scopes: `app_mentions:read`, `chat:write`, `channels:read`, `channels:history`, `groups:history`, `im:history`, `im:write`, `mpim:history`, `users:read`, `users:read.email`
- Notes:
  - Channel posts are only delivered where the bot is a member. For ad‑hoc channels without the bot, use the Context Menu (shortcut) or Slash Commands.
  - Channel-post mode uses self-only context (no conversation history) by design; mentions/DMs keep recent-context fetch.

Details
1) Enable Slack headend
- Ensure `.ai-agent.json` has a `slack` section (env placeholders permitted):

```json
{
  "slack": {
    "enabled": true,
    "mentions": true,
    "dms": true,
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "openerTone": "random",
    "routing": {
      "default": {
        "agent": "neda.ai",
        "engage": ["mentions", "dms"],
        "promptTemplates": {
          "channelPost": "You are responding to a channel post in {channel.name}. Message: {text}"
        },
        "contextPolicy": { "channelPost": "selfOnly" }
      },
      "rules": [
        {
          "channels": ["#support", "#sla"],
          "agent": "neda.ai",
          "engage": ["channel-posts"],
          "promptTemplates": {
            "channelPost": "Support Engineer in {channel.name}. Be concise. Message: {text}"
          }
        }
      ],
      "deny": [
        { "channels": ["#customer*"], "engage": ["mentions", "dms", "channel-posts"] }
      ]
    }
  }
}
```

2) Routing rules and precedence
- `deny` → first matched rule in `rules` → `default` (if `engage` includes the kind)
- `channels` accept `#names`, IDs (`C…`/`G…`), and wildcards (`*`, `?`). Name matching is case-insensitive.
- `engage`: any of `mentions`, `channel-posts`, `dms`.
- `promptTemplates`: optional per kind (`mention`, `dm`, `channelPost`); variables:
  - `{text}`, `{user.id}`, `{user.label}`, `{channel.id}`, `{channel.name}`, `{ts}`, `{message.url}` (permalink when available)
- Context policy:
  - `channelPost` supports `selfOnly` (default), `previousOnly`, or `selfAndPrevious`. Mentions/DMs continue using recent context.

3) Running
- Start ai-agent with Slack and API:

```bash
ai-agent server ./neda/neda.ai --slack --api --verbose
```

-------------------------------------------------------------------------------

## Slash Commands

Summary
- What it does:
  - Handles `/neda ...` invocations in any channel or DM.
  - Routes via the same per-channel rules and templates. Uses self-only context.
- Required config:
  - Public HTTPS endpoint for Slack to POST (e.g., `/slack/commands`). A tunnel (ngrok/Cloudflare Tunnel) is fine for dev.
  - `.ai-agent.json` → `slack.signingSecret`: `${SLACK_SIGNING_SECRET}`.
  - Slack manifest: add `commands` scope and define the slash command (`/neda`) with Request URL.

Details
1) Configure server endpoint
- ai-agent exposes `POST /slack/commands` (Express) and validates Slack signatures.
- Add to `.ai-agent.json` under `slack`:

```json
{
  "slack": {
    "signingSecret": "${SLACK_SIGNING_SECRET}"
  }
}
```

2) Manifest and scopes
- Add `commands` to bot scopes and define the slash command (see Manifest section below).

3) Local testing (no production yet)
- Use a tunnel so Slack can reach your local port:

```bash
# Example using cloudflared
cloudflared tunnel --url http://localhost:8080
# or ngrok
ngrok http 8080
```

- Configure the Slash Command Request URL to: `https://<your-tunnel-domain>/slack/commands`.

4) Curl-style signed test (optional)
- Without Slack, simulate a signed POST to your local endpoint:

```bash
export SLACK_SIGNING_SECRET=your-secret
data='token=fake&team_id=T123&team_domain=local&channel_id=C123&channel_name=test&user_id=U123&user_name=me&command=%2Fneda&text=hello+world&response_url=https%3A%2F%2Fexample.com%2Fresp'
ts=$(date +%s)
base="v0:$ts:$data"
sig="v0=$(echo -n "$base" | openssl dgst -sha256 -hmac "$SLACK_SIGNING_SECRET" -hex | sed 's/^.* //')"
curl -i -X POST http://localhost:8080/slack/commands \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "X-Slack-Request-Timestamp: $ts" \
  -H "X-Slack-Signature: $sig" \
  --data "$data"
```

-------------------------------------------------------------------------------

## Context Menu (Message Shortcut: "Ask Neda")

Summary
- What it does:
  - Adds a message shortcut “Ask Neda” in the message actions menu.
  - Works entirely over Socket Mode; no public endpoint required.
  - Routes like `channel-posts` with self-only context, using your per-channel templates; if the bot isn’t a member, continues in DM with a permalink.
- Required config:
  - Interactivity enabled (already enabled in manifest).
  - Shortcut defined in manifest with `callback_id: ask_neda`.

Details
- User flow:
  - User clicks “Ask Neda” on a message → app ACKs → applies routing → replies in-thread if possible, else DMs the user.
- Template variables available are the same as channel posts, including `{message.url}`.

-------------------------------------------------------------------------------

## Slack Manifest Updates

Scopes
- Ensure the following bot scopes are present:
  - `app_mentions:read`, `chat:write`, `channels:read`, `channels:history`, `groups:history`, `im:history`, `im:write`, `mpim:history`, `users:read`, `users:read.email`, `commands`

Events
- Bot events (Socket Mode):
  - `app_mention`, `message.im`, `message.channels`, `message.groups`

Interactivity
- Must be enabled (true) for buttons/shortcuts/modals.

Shortcut
- Add message shortcut (works with Socket Mode):
  - name: "Ask Neda"
  - type: "message"
  - callback_id: "ask_neda"

Slash Command
- Add a slash command (requires public HTTPS):
  - command: "/neda"
  - request URL: `https://<your-domain>/slack/commands`
  - description: "Ask Neda"
  - usage hint: "<prompt>"

Applying the manifest
1) From Slack: https://api.slack.com/apps → select your app → "App Manifest" → "Edit Manifest".
2) Update JSON to include the scopes, events, shortcut, and (optionally) slash command.
3) Save changes. If scopes changed, reinstall the app: “Install App to Workspace”.
4) Copy credentials to `.ai-agent.env` (do not commit secrets):
   - `SLACK_BOT_TOKEN` (xoxb-...)
   - `SLACK_APP_TOKEN` (xapp-...) — for Socket Mode
   - `SLACK_SIGNING_SECRET` — for slash commands

-------------------------------------------------------------------------------

## Routing: Overlapping Wildcards, Deny, and Templates

When multiple rules could match, precedence is:
- deny rules first (if any deny matches, the event is ignored)
- then the first rule in `rules[]` that matches
- then `default` (if its `engage` includes the kind)

Example with overlapping wildcards and deny:

```json
{
  "slack": {
    "routing": {
      "deny": [
        { "channels": ["#support-internal"], "engage": ["channel-posts"] }
      ],
      "rules": [
        { "channels": ["#support-urgent", "#support-*"] , "agent": "neda.ai", "engage": ["channel-posts"],
          "promptTemplates": { "channelPost": "Urgent support handling for {channel.name}. Message: {text}" } }
      ],
      "default": { "agent": "neda.ai", "engage": ["mentions", "dms"] }
    }
  }
}
```

Notes:
- `#support-internal` is denied for `channel-posts` regardless of later rules.
- `#support-urgent` matches before `#support-*` because it appears first.
- Other `#support-*` channels match the wildcard rule.

Template variables you can use in `promptTemplates`:
- `{text}`, `{user.id}`, `{user.label}`, `{channel.id}`, `{channel.name}`, `{ts}`, `{message.url}`

Context policies:
- `channelPost`: `selfOnly` (default), `previousOnly`, `selfAndPrevious`
- Mentions/DMs keep recent context prefetch and are not affected by `channelPost` policy.

## Troubleshooting

- No responses on channel posts
  - Ensure the bot is invited to the channel; otherwise, use the message shortcut or slash commands.

- Mention not triggering
  - Verify `app_mentions:read` scope and `app_mention` event are present. Ensure Socket Mode is enabled.

- Slash command 401/invalid signature
  - Verify `SLACK_SIGNING_SECRET` and that the Request URL points to your public endpoint.
  - Check system time (signature window is ±5 minutes).

- Shortcut not appearing
  - Ensure the shortcut exists in the manifest and Interactivity is enabled. Reinstall app.

- Routing not applied as expected
  - Confirm channel names/IDs and wildcard patterns. Remember name matching is case‑insensitive; ID matching is exact.

-------------------------------------------------------------------------------

## Reference: Template Variables

- `{text}` — user-provided text (message or slash command text)
- `{user.id}` — Slack user ID (e.g., U123…)
- `{user.label}` — human label: real/display name best effort
- `{channel.id}` — Slack channel ID (e.g., C123… or G123…)
- `{channel.name}` — channel name (best effort; not always available)
- `{ts}` — human-readable timestamp
- `{message.url}` — deep link to the selected message (when available)
