# Neda AI CRM Agent - User Configuration Guide

Neda is an AI-powered assistant that provides comprehensive business intelligence, customer research, and data analysis capabilities. It's deployed at **10.20.1.106** and accessible through multiple integration methods.

## Table of Contents

- [Integration Options](#integration-options)
- [Available Agents](#available-agents)
- [Integration Methods](#integration-methods)
  - [Slack](#slack)
  - [Open WebUI](#open-webui)
  - [MCP Tool Integration](#mcp-tool-integration)
  - [OpenAI-Compatible LLM API](#openai-compatible-llm-api)
  - [Anthropic-Compatible LLM API](#anthropic-compatible-llm-api)
  - [REST API Integration](#rest-api-integration)
- [Client Configuration](#client-configuration)
  - [Claude Code](#claude-code)
  - [Claude Desktop](#claude-desktop)
  - [Codex CLI](#codex-cli)
  - [VS Code (GitHub Copilot)](#vs-code-github-copilot)
  - [Python Applications (OpenAI SDK)](#python-applications-openai-sdk)
  - [JavaScript/TypeScript Applications (OpenAI SDK)](#javascripttypescript-applications-openai-sdk)
  - [Python Applications (Anthropic SDK)](#python-applications-anthropic-sdk)
  - [Shell Scripts / curl (REST API)](#shell-scripts--curl-rest-api)
- [Testing & Verification](#testing--verification)
- [Troubleshooting](#troubleshooting)
- [Support](#support)

---

## Integration Options

Neda supports six integration methods:

| Integration Type | Best For | Endpoints / Access |
|-----------------|----------|-------------------|
| **Slack** | Team collaboration, quick queries | `@Neda` mentions or "Ask Neda" context menu |
| **Open WebUI** | Web-based chat interface | `https://10.20.1.106/` (sign-up/sign-in required) |
| **MCP Tool** | AI assistants (Claude Code, Claude Desktop, Codex CLI) | `http://10.20.1.106:8801/mcp` (HTTP)<br>`http://10.20.1.106:8802/mcp/sse` (SSE)<br>`ws://10.20.1.106:8803/mcp` (WebSocket) |
| **OpenAI-Compatible API** | Applications using OpenAI SDK | `http://10.20.1.106:8804/v1` |
| **Anthropic-Compatible API** | Applications using Anthropic SDK | `http://10.20.1.106:8805/v1` |
| **REST API** | Custom integrations, curl commands | `http://10.20.1.106:8800/v1/{agent}` |

---

## Available Agents

Neda provides 24 specialized agents for different business intelligence needs.

**Note:** When Neda is exposed as an MCP tool, all agents below are available as individual MCP tools that can be called directly.

### Main Orchestrator

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `neda` | Main orchestrator - coordinates all other agents and has access to all their capabilities combined | "Tell me everything about customer Acme Corp" |

### Sales & CRM Agents

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `company` | Company research and business intelligence | "Research Microsoft" |
| `company-tech` | Technology stack identification | "What tech does spotify.com use?" |
| `contact` | Professional profile research | "Find info about John Smith at Microsoft" |
| `hubspot` | HubSpot CRM data extraction | "Show HubSpot data for acme.com" |
| `stripe` | Stripe payment and billing intelligence | "Billing info for acme.com" |
| `fireflies` | Meeting transcripts analysis | "Meetings with Acme Corp last 30 days" |

### Analytics & Data Agents

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `bigquery` | Netdata Cloud production data queries | "Infrastructure scale for space xyz" |
| `posthog` | PostHog product usage analytics | "User activity for john@acme.com" |
| `executive` | Business analytics (ARR, MRR, growth, churn) | "What is our current ARR?" |
| `ga` | Google Analytics traffic insights | "Traffic trends last 30 days" |
| `cloudflare` | Website performance and security analytics | "Cloudflare stats for learn.netdata.cloud" |
| `encharge` | Email marketing analytics | "Email campaigns performance" |
| `gsc` | Google Search Console SEO data | "Top pages on learn.netdata.cloud" |

### Technical & Support Agents

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `github` | Netdata GitHub repository intelligence (public and private repos) | "Find issues about ml_anomaly_score" |
| `freshdesk` | Customer support tickets | "Open tickets for Acme Corp" |
| `netdata` | Infrastructure health monitoring | "Production systems health check" |
| `source-code` | Netdata source code analysis | "How does TROUBLESHOOT button work?" |

### Research & Web Agents

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `web-research` | Deep investigative research on any subject | "Latest observability trends 2024" |
| `web-search` | Perform web searches | "Search for AI news" |
| `web-fetch` | Fetch and extract content from URLs | JSON input with url and extract fields |
| `reddit` | Search user comments on Reddit | "What do users say about Netdata?" |

### Messaging & Strategy Agents

| Agent | Purpose | Example Query |
|-------|---------|---------------|
| `product-messaging` | AI Product Messaging Expert for positioning | "Position Netdata for healthcare industry" |
| `neda-second-opinion` | Critical analysis second opinion | "Analyze this strategy" |

### Output Formats

All agents support these output formats via the `format` parameter:

| Format | Description | Use Case |
|--------|-------------|----------|
| `markdown` | GitHub Flavored Markdown | Human-readable, default format |
| `markdown+mermaid` | Markdown with Mermaid diagrams | Documentation with visualizations |
| `json` | Structured JSON | API integrations, parsing |
| `pipe` | Plain text | Shell scripts, pipelines |
| `tty` | ANSI colored text | Terminal output |
| `slack-block-kit` | Slack Block Kit payload | Slack integrations |

---

# Integration Methods

## Slack

**What is it?** Neda is available as a Slack bot in your workspace for quick access and team collaboration.

**How to access:**
- **@Neda mentions**: Mention `@Neda` in any channel where Neda is invited, or send direct messages
- **Context menu**: Right-click on any message → "Ask Neda" to analyze or respond to that message

**Setup:**
1. Invite Neda to a channel: `/invite @Neda`
2. Ask questions by mentioning: `@Neda tell me about customer Acme Corp`
3. Use context menu on messages for quick analysis

**Use cases:**
- Quick customer intelligence queries during team discussions
- Share research findings with team members
- Collaborative data analysis in channels
- Ad-hoc questions without leaving Slack

---

## Open WebUI

**What is it?** A web-based chat interface for interacting with Neda and all its agents.

**Access:** `https://10.20.1.106/`

**Setup:**
1. Navigate to `https://10.20.1.106/`
2. Sign up for an account (first-time users)
3. Sign in with your credentials

**How to use:**
1. Select a model from the dropdown:
   - `neda` - Main orchestrator with access to all capabilities
   - Any specific agent name (e.g., `company`, `web-research`, `bigquery`)
2. Type your query in the chat interface
3. Receive responses in real-time

**Note:** Open WebUI uses Neda's OpenAI-compatible API (port 8804) behind the scenes.

**Use cases:**
- Browser-based access without client configuration
- Visual chat interface for interactive queries
- Testing different agents and comparing responses
- Sharing access with team members via web link

---

## MCP Tool Integration

**What is it?** Model Context Protocol (MCP) allows AI assistants like Claude Code to use Neda as an external tool during conversations.

**How it works:** Your AI assistant calls Neda agents as needed, automatically passing context and receiving structured responses.

**Supported transports:**
- **SSE (Server-Sent Events)**: `http://10.20.1.106:8802/mcp/sse` - Recommended for better streaming support
- **HTTP**: `http://10.20.1.106:8801/mcp` - Standard request/response
- **WebSocket**: `ws://10.20.1.106:8803/mcp` - Bidirectional streaming

**Security note:** Neda runs on HTTP/WS (not HTTPS/WSS) within the secure private network at 10.20.1.106. When using `mcp-remote`, you must include the `--allow-http` flag. This flag should only be used in trusted private networks.

---

## OpenAI-Compatible LLM API

**What is it?** Neda exposes an OpenAI-compatible chat completions API.

**Endpoint:** `http://10.20.1.106:8804/v1`

**Authentication:** Any non-empty API key (e.g., `"your-key-here"`)

**Model names:** Use agent names as model names:
- `neda` - Main CRM bot
- `company` - Company research
- `web-research` - Web research
- (see Available Agents section)

**Use cases:**
- Drop-in replacement for OpenAI in existing applications
- LangChain, LlamaIndex integrations
- Custom applications using OpenAI SDK

---

## Anthropic-Compatible LLM API

**What is it?** Neda exposes an Anthropic Claude-compatible messages API.

**Endpoint:** `http://10.20.1.106:8805/v1/messages`

**Authentication:** Any non-empty API key via `x-api-key` header

**Required headers:**
- `x-api-key: your-key-here`
- `anthropic-version: 2023-06-01`
- `content-type: application/json`

**Model names:** Same agent names as OpenAI API

**Use cases:**
- Applications built for Anthropic Claude API
- Drop-in replacement for Claude in existing code

---

## REST API Integration

**What is it?** Direct HTTP GET requests to query specific agents.

**Base URL:** `http://10.20.1.106:8800`

**Endpoint pattern:** `GET /v1/{agent}?q={query}&format={format}`

**Parameters:**
- `q` (required): Your query (URL-encoded)
- `format` (optional): Response format - `markdown`, `json`, `pipe`, `tty`, `slack-block-kit` (default: `markdown`)

**Use cases:**
- Shell scripts and automation
- Simple integrations without SDK dependencies
- Quick testing with curl

---

# Client Configuration

## Claude Code

Claude Code has native support for remote MCP servers.

### Method 1: Command Line (Recommended)

Add Neda using the CLI:

```bash
# SSE transport (recommended)
claude mcp add --transport sse neda http://10.20.1.106:8802/mcp/sse

# OR HTTP transport
claude mcp add --transport http neda http://10.20.1.106:8801/mcp
```

Verify the connection:
```bash
claude mcp list
claude mcp get neda
```

### Method 2: Configuration File

Edit `~/.claude.json`:

**SSE transport (recommended):**
```json
{
  "mcpServers": {
    "neda": {
      "type": "sse",
      "url": "http://10.20.1.106:8802/mcp/sse"
    }
  }
}
```

**HTTP transport:**
```json
{
  "mcpServers": {
    "neda": {
      "type": "http",
      "url": "http://10.20.1.106:8801/mcp"
    }
  }
}
```

**After editing:** Restart Claude Code.

### Usage

Once configured, simply mention Neda in your conversations:

```
@neda tell me about customer Acme Corp
```

Or let Claude automatically use Neda when relevant:

```
What's the technology stack of spotify.com?
```

---

## Claude Desktop

Claude Desktop requires `mcp-remote` to connect to remote MCP servers.

### Configuration

Edit `~/.config/claude/config.json` (macOS/Linux) or `%APPDATA%\Claude\config.json` (Windows):

**SSE transport (recommended):**
```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://10.20.1.106:8802/mcp/sse",
        "--allow-http"
      ]
    }
  }
}
```

**HTTP transport:**
```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://10.20.1.106:8801/mcp",
        "--allow-http"
      ]
    }
  }
}
```

**After editing:** Restart Claude Desktop.

**Important:** The `--allow-http` flag is required because Neda uses HTTP (not HTTPS) within the secure internal network. Only use this in trusted private networks.

### First-time Setup

On first use, `mcp-remote` will be downloaded automatically via `npx`. This may take a few seconds.

---

## Codex CLI

Codex CLI uses the same `mcp-remote` approach as Claude Desktop.

### Configuration

Edit `~/.codex/config.json`:

**SSE transport (recommended):**
```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://10.20.1.106:8802/mcp/sse",
        "--allow-http"
      ]
    }
  }
}
```

**HTTP transport:**
```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://10.20.1.106:8801/mcp",
        "--allow-http"
      ]
    }
  }
}
```

**After editing:** Restart or reload Codex configuration.

---

## VS Code (GitHub Copilot)

VS Code with GitHub Copilot supports MCP servers.

### Configuration

Add to your VS Code settings (`.vscode/settings.json` or User Settings):

```json
{
  "github.copilot.advanced": {
    "mcp": {
      "servers": {
        "neda": {
          "command": "npx",
          "args": [
            "-y",
            "mcp-remote",
            "http://10.20.1.106:8802/mcp/sse",
            "--allow-http"
          ]
        }
      }
    }
  }
}
```

**After editing:** Reload VS Code window.

---

## Python Applications (OpenAI SDK)

### Installation

```bash
pip install openai
```

### Example Code

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-key-here",  # Any non-empty string works
    base_url="http://10.20.1.106:8804/v1"
)

# Query the main neda agent
response = client.chat.completions.create(
    model="neda",
    messages=[
        {"role": "user", "content": "Tell me about customer Acme Corp"}
    ]
)
print(response.choices[0].message.content)

# Query a specific agent
response = client.chat.completions.create(
    model="company",
    messages=[
        {"role": "user", "content": "Research Tesla"}
    ]
)
print(response.choices[0].message.content)
```

---

## JavaScript/TypeScript Applications (OpenAI SDK)

### Installation

```bash
npm install openai
```

### Example Code

```typescript
import OpenAI from 'openai';

const openai = new OpenAI({
  apiKey: 'your-key-here',  // Any non-empty string works
  baseURL: 'http://10.20.1.106:8804/v1',
});

// Query the main neda agent
const response = await openai.chat.completions.create({
  model: 'neda',
  messages: [{ role: 'user', content: 'Tell me about customer Acme Corp' }],
});
console.log(response.choices[0].message.content);

// Query a specific agent
const companyResponse = await openai.chat.completions.create({
  model: 'company',
  messages: [{ role: 'user', content: 'Research Tesla' }],
});
console.log(companyResponse.choices[0].message.content);
```

---

## Python Applications (Anthropic SDK)

### Installation

```bash
pip install anthropic
```

### Example Code

```python
from anthropic import Anthropic

client = Anthropic(
    api_key="your-key-here",  # Any non-empty string works
    base_url="http://10.20.1.106:8805/v1"
)

# Query the main neda agent
message = client.messages.create(
    model="neda",
    max_tokens=4096,
    messages=[
        {"role": "user", "content": "Research company Netdata"}
    ]
)
print(message.content[0].text)

# Query a specific agent
company_message = client.messages.create(
    model="company",
    max_tokens=4096,
    messages=[
        {"role": "user", "content": "Research Tesla"}
    ]
)
print(company_message.content[0].text)
```

---

## Shell Scripts / curl (REST API)

### Health Check

```bash
curl http://10.20.1.106:8800/health
```

Response:
```json
{"status": "ok"}
```

### Query Agents

**Basic syntax:**
```bash
curl -G "http://10.20.1.106:8800/v1/{agent}" \
  --data-urlencode "q=your query here" \
  --data-urlencode "format=markdown"
```

**Examples:**

```bash
# Main Neda CRM bot
curl -G "http://10.20.1.106:8800/v1/neda" \
  --data-urlencode "q=Tell me about customer Acme Corp" \
  --data-urlencode "format=markdown"

# Company research
curl -G "http://10.20.1.106:8800/v1/company" \
  --data-urlencode "q=research Microsoft" \
  --data-urlencode "format=json"

# Technology stack
curl -G "http://10.20.1.106:8800/v1/company-tech" \
  --data-urlencode "q=netflix.com" \
  --data-urlencode "format=markdown"

# Web research
curl -G "http://10.20.1.106:8800/v1/web-research" \
  --data-urlencode "q=Latest observability trends 2024" \
  --data-urlencode "format=markdown"

# Executive analytics
curl -G "http://10.20.1.106:8800/v1/executive" \
  --data-urlencode "q=What is our current ARR?" \
  --data-urlencode "format=json"
```

### Response Format

**Success:**
```json
{
  "success": true,
  "output": "The agent's response text in requested format",
  "finalReport": {
    // Structured data if available
  }
}
```

**Error:**
```json
{
  "success": false,
  "output": "",
  "finalReport": null,
  "error": "error_code_here"
}
```

### Shell Script Helper

Save as `neda-query.sh`:

```bash
#!/bin/bash
# Usage: ./neda-query.sh agent "your query" [format]

AGENT="${1}"
QUERY="${2}"
FORMAT="${3:-markdown}"

curl -G "http://10.20.1.106:8800/v1/${AGENT}" \
  --data-urlencode "q=${QUERY}" \
  --data-urlencode "format=${FORMAT}"
```

Usage:
```bash
chmod +x neda-query.sh
./neda-query.sh company "Research Tesla" json
./neda-query.sh web-research "AI trends 2024" markdown
```

---

# Testing & Verification

## Quick Connection Tests

### Test REST API
```bash
curl http://10.20.1.106:8800/health
# Expected: {"status": "ok"}
```

### Test MCP Endpoint
```bash
curl -X POST http://10.20.1.106:8801/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0.0"}}, "id": 1}'
# Expected: JSON-RPC response with server capabilities
```

### Test OpenAI-Compatible API
```bash
curl http://10.20.1.106:8804/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "neda",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
# Expected: OpenAI-compatible chat completion response
```

### Test Anthropic-Compatible API
```bash
curl http://10.20.1.106:8805/v1/messages \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "neda",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 1000
  }'
# Expected: Anthropic-compatible messages response
```

## Verify MCP Client Connection

### Claude Code
```bash
claude mcp list
# Should show "neda" in the list

claude mcp get neda
# Should show connection details and available tools
```

### Test Query
Once configured, test with a simple query:

**Claude Code / Claude Desktop / Codex:**
```
@neda Hello, are you working?
```

**REST API:**
```bash
curl -G "http://10.20.1.106:8800/v1/neda" \
  --data-urlencode "q=Hello, are you working?" \
  --data-urlencode "format=markdown"
```

---

# Troubleshooting

## Common Issues

### MCP Connection Failed

**Symptom:** "Failed to connect to MCP server" or timeout errors

**Solutions:**
1. Verify network connectivity: `ping 10.20.1.106`
2. Check if services are running: `curl http://10.20.1.106:8800/health`
3. For `mcp-remote`, ensure `--allow-http` flag is present
4. Check client configuration file syntax (valid JSON)
5. Restart your AI assistant after configuration changes

### "401 Unauthorized" or Authentication Errors

**Symptom:** Authentication failed when using LLM APIs

**Solution:** For Neda's APIs, any non-empty string works as API key. Ensure you're providing something like `"your-key-here"` or `"test-key"`.

### Slow Responses

**Symptom:** Queries take very long or timeout

**Explanation:** Some agents (especially research agents) perform extensive web searches and analysis, which can take 30-60 seconds or more.

**Solutions:**
- Be patient, especially with `web-research`, `company`, and `company-tech` agents
- Use more specific queries to reduce research scope
- For time-sensitive queries, use faster agents like `hubspot`, `bigquery`, or `stripe`

### "Agent not found" Error

**Symptom:** Error message indicating agent doesn't exist

**Solution:** Check agent name spelling. Use exact names from the Available Agents table (case-sensitive).

### No Tools Available in MCP Client

**Symptom:** Neda server connected but no tools are visible

**Solutions:**
1. Check MCP initialization in client logs
2. Verify server protocol version compatibility
3. Restart the AI assistant
4. Check if Neda service is running: `curl http://10.20.1.106:8800/health`

---

# Support

For issues not covered in this guide:

1. Check Neda service status: `curl http://10.20.1.106:8800/health`
2. Test with curl directly to isolate client vs server issues
3. Verify your network can reach 10.20.1.106 (internal network only)
4. Contact the Neda administrator for service-level issues
