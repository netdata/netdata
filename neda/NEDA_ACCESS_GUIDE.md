# Neda AI CRM Agent - Complete Access Guide

Neda is available at **10.20.4.205** and provides multiple interfaces for integration. This guide covers all methods to access Neda's capabilities.

## Available Neda Agents

- `neda` - Main CRM bot for customer and prospect intelligence
- `company` - Company research and intelligence
- `company-tech` - Technology stack identification
- `contact` - Professional profile research
- `stripe` - Stripe company intelligence
- `hubspot` - HubSpot CRM data extraction
- `web-research` - Elite investigative web research
- `fireflies` - Meeting transcripts analysis
- `bigquery` - BigQuery production data queries
- `posthog` - PostHog product usage analytics
- `github` - Netdata GitHub repository intelligence
- `freshdesk` - Customer support tickets
- `executive` - Business analytics (ARR, MRR, growth)
- `netdata` - Infrastructure health monitoring
- `source-code` - Netdata source code analysis
- `gsc` - Google Search Console SEO data
- `ga` - Google Analytics traffic insights
- `cloudflare` - Website performance and security
- `encharge` - Email marketing analytics
- `neda-second-opinion` - Critical analysis second opinion

## 1. Using Neda as an MCP Tool

### With Claude Code (Supports Remote MCP)

Add to your Claude Code configuration (`claude_config.json`):

```json
{
  "mcpServers": {
    "neda": {
      "url": "http://10.20.4.205:8801/mcp",
      "transport": "http"
    }
  }
}
```

Or use WebSocket for persistent connection:

```json
{
  "mcpServers": {
    "neda": {
      "url": "ws://10.20.4.205:8803/mcp",
      "transport": "websocket"
    }
  }
}
```

Or use Server-Sent Events (SSE):

```json
{
  "mcpServers": {
    "neda": {
      "url": "http://10.20.4.205:8802/mcp/sse",
      "transport": "sse"
    }
  }
}
```

### With Claude Desktop (Via mcp-remote)

Since Claude Desktop doesn't support remote MCP directly, use `mcp-remote`:

```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/mcp-remote",
        "connect",
        "--transport",
        "http",
        "http://10.20.4.205:8801/mcp"
      ]
    }
  }
}
```

For WebSocket transport:

```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/mcp-remote",
        "connect",
        "--transport",
        "websocket",
        "ws://10.20.4.205:8803/mcp"
      ]
    }
  }
}
```

### With Codex CLI (Via mcp-remote)

Add to your Codex configuration (`~/.codex/config.json`):

```json
{
  "mcpServers": {
    "neda": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/mcp-remote",
        "connect",
        "--transport",
        "http",
        "http://10.20.4.205:8801/mcp"
      ]
    }
  }
}
```

## 2. Using Neda as an LLM

### OpenAI-Compatible API

Configure your OpenAI client to use Neda:

```python
# Python example
from openai import OpenAI

client = OpenAI(
    api_key="your-key-here",  # Any non-empty string
    base_url="http://10.20.4.205:8804/v1"
)

response = client.chat.completions.create(
    model="neda",  # or any specific agent like "company", "web-research"
    messages=[
        {"role": "user", "content": "Tell me about customer Acme Corp"}
    ]
)
print(response.choices[0].message.content)
```

```javascript
// JavaScript/TypeScript example
import OpenAI from 'openai';

const openai = new OpenAI({
  apiKey: 'your-key-here', // Any non-empty string
  baseURL: 'http://10.20.4.205:8804/v1',
});

const response = await openai.chat.completions.create({
  model: 'neda', // or "company", "web-research", etc.
  messages: [{ role: 'user', content: 'Tell me about customer Acme Corp' }],
});
console.log(response.choices[0].message.content);
```

### Anthropic-Compatible API

```python
# Python example
import httpx

response = httpx.post(
    "http://10.20.4.205:8805/v1/messages",
    headers={
        "x-api-key": "your-key-here",  # Any non-empty string
        "anthropic-version": "2023-06-01",
        "content-type": "application/json"
    },
    json={
        "model": "neda",  # or any specific agent
        "messages": [
            {"role": "user", "content": "Research company Netdata"}
        ],
        "max_tokens": 4096
    }
)
print(response.json()["content"][0]["text"])
```

```javascript
// JavaScript/TypeScript example
const response = await fetch('http://10.20.4.205:8805/v1/messages', {
  method: 'POST',
  headers: {
    'x-api-key': 'your-key-here', // Any non-empty string
    'anthropic-version': '2023-06-01',
    'content-type': 'application/json',
  },
  body: JSON.stringify({
    model: 'neda', // or "company", "web-research", etc.
    messages: [{ role: 'user', content: 'Research company Netdata' }],
    max_tokens: 4096,
  }),
});
const data = await response.json();
console.log(data.content[0].text);
```

## 3. Using Neda via REST API

### Health Check

```bash
curl http://10.20.4.205:8800/health
```

Response:
```json
{"status": "ok"}
```

### Query Agents

#### Main Neda CRM Bot

```bash
# Get customer information
curl -G "http://10.20.4.205:8800/v1/neda" \
  --data-urlencode "q=Tell me everything about customer Acme Corp" \
  --data-urlencode "format=markdown"

# Search for prospects
curl -G "http://10.20.4.205:8800/v1/neda" \
  --data-urlencode "q=Find companies using Kubernetes in healthcare" \
  --data-urlencode "format=json"
```

#### Company Research

```bash
# Research a company
curl -G "http://10.20.4.205:8800/v1/company" \
  --data-urlencode "q=research Microsoft" \
  --data-urlencode "format=markdown"

# Research by email domain
curl -G "http://10.20.4.205:8800/v1/company" \
  --data-urlencode "q=john@acmecorp.com" \
  --data-urlencode "format=json"
```

#### Technology Stack Research

```bash
# Identify tech stack
curl -G "http://10.20.4.205:8800/v1/company-tech" \
  --data-urlencode "q=netflix.com" \
  --data-urlencode "format=markdown"
```

#### Contact Research

```bash
# Research a person
curl -G "http://10.20.4.205:8800/v1/contact" \
  --data-urlencode "q=John Smith at Microsoft" \
  --data-urlencode "format=markdown"
```

#### Web Research

```bash
# General research query
curl -G "http://10.20.4.205:8800/v1/web-research" \
  --data-urlencode "q=Latest trends in observability and monitoring 2024" \
  --data-urlencode "format=markdown"
```

#### Stripe Intelligence

```bash
# Get Stripe data for a company
curl -G "http://10.20.4.205:8800/v1/stripe" \
  --data-urlencode "q=acmecorp.com" \
  --data-urlencode "format=json"
```

#### HubSpot Data

```bash
# Extract HubSpot CRM data
curl -G "http://10.20.4.205:8800/v1/hubspot" \
  --data-urlencode "q=acmecorp.com" \
  --data-urlencode "format=markdown"
```

#### Meeting Analysis

```bash
# Analyze meeting transcripts
curl -G "http://10.20.4.205:8800/v1/fireflies" \
  --data-urlencode "q=Acme Corp meetings last 30 days" \
  --data-urlencode "format=markdown"
```

#### BigQuery Analytics

```bash
# Query production data
curl -G "http://10.20.4.205:8800/v1/bigquery" \
  --data-urlencode "q=Show infrastructure scale for space xyz last 7 days" \
  --data-urlencode "format=json"
```

#### PostHog Usage Analytics

```bash
# Analyze product usage
curl -G "http://10.20.4.205:8800/v1/posthog" \
  --data-urlencode "q=acme@corp.com usage patterns last month" \
  --data-urlencode "format=markdown"
```

#### Executive Analytics

```bash
# Business metrics
curl -G "http://10.20.4.205:8800/v1/executive" \
  --data-urlencode "q=What is our current ARR and growth rate?" \
  --data-urlencode "format=markdown"

# Revenue analytics
curl -G "http://10.20.4.205:8800/v1/executive" \
  --data-urlencode "q=Show me churn analysis for Q3 2024" \
  --data-urlencode "format=json"
```

### Response Format

All REST API responses follow this structure:

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

### Python Client Example

```python
import requests
from urllib.parse import urlencode

class NedaClient:
    def __init__(self, base_url="http://10.20.4.205:8800"):
        self.base_url = base_url

    def query(self, agent, prompt, format="markdown"):
        params = {"q": prompt, "format": format}
        response = requests.get(f"{self.base_url}/v1/{agent}", params=params)
        return response.json()

    def health_check(self):
        response = requests.get(f"{self.base_url}/health")
        return response.json()

# Usage
client = NedaClient()

# Check health
print(client.health_check())

# Query various agents
customer_info = client.query("neda", "Tell me about customer Acme Corp")
company_research = client.query("company", "Research Tesla")
tech_stack = client.query("company-tech", "spotify.com")
web_research = client.query("web-research", "Latest AI trends 2024")
```

### JavaScript/TypeScript Client Example

```typescript
class NedaClient {
  private baseUrl: string;

  constructor(baseUrl = 'http://10.20.4.205:8800') {
    this.baseUrl = baseUrl;
  }

  async query(agent: string, prompt: string, format = 'markdown') {
    const params = new URLSearchParams({ q: prompt, format });
    const response = await fetch(`${this.baseUrl}/v1/${agent}?${params}`);
    return response.json();
  }

  async healthCheck() {
    const response = await fetch(`${this.baseUrl}/health`);
    return response.json();
  }
}

// Usage
const client = new NedaClient();

// Check health
console.log(await client.healthCheck());

// Query various agents
const customerInfo = await client.query('neda', 'Tell me about customer Acme Corp');
const companyResearch = await client.query('company', 'Research Tesla');
const techStack = await client.query('company-tech', 'spotify.com');
const webResearch = await client.query('web-research', 'Latest AI trends 2024');
```

## Quick Testing Commands

```bash
# Test MCP HTTP endpoint
curl -X POST http://10.20.4.205:8801/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0.0"}}, "id": 1}'

# Test REST API
curl http://10.20.4.205:8800/health

# Test OpenAI-compatible endpoint
curl http://10.20.4.205:8804/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "neda",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# Test Anthropic-compatible endpoint
curl http://10.20.4.205:8805/v1/messages \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "neda",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 1000
  }'
```

## Support Formats

All agents support the following output formats via the `format` parameter:
- `markdown` - GitHub Flavored Markdown
- `json` - Structured JSON response
- `pipe` - Plain text without formatting
- `tty` - TTY-compatible with ANSI colors
- `slack-block-kit` - Slack Block Kit payload