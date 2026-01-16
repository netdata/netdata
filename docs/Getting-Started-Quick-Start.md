# Quick Start

Get a working agent in 5 minutes.

---

## Step 1: Create Configuration

Create `.ai-agent.json`:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "defaults": {
    "temperature": 0.7
  }
}
```

Set your API key:

```bash
export OPENAI_API_KEY="sk-..."
```

---

## Step 2: Create Your First Agent

Create `hello.ai`:

```yaml
#!/usr/bin/env ai-agent
---
description: A friendly greeting agent
models:
  - openai/gpt-4o-mini
maxTurns: 3
---
You are a friendly assistant. Greet the user warmly and help them with their questions.
```

---

## Step 3: Run It

```bash
ai-agent --agent hello.ai "Hello, who are you?"
```

Output:
```
Hello! I'm a friendly AI assistant. I'm here to help you with any questions
you might have. How can I assist you today?
```

---

## Step 4: Add Tools (Optional)

Update your config to add MCP tools:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"]
    }
  }
}
```

Update `hello.ai` to use tools:

```yaml
#!/usr/bin/env ai-agent
---
description: A file-aware assistant
models:
  - openai/gpt-4o-mini
tools:
  - filesystem
maxTurns: 10
---
You are a helpful assistant that can read and write files.
```

---

## Step 5: Run as REST API

```bash
ai-agent --agent hello.ai --api 8080
```

Then call it:

```bash
curl "http://localhost:8080/v1/hello?q=Hello"
```

---

## What's Next?

- [First Agent Tutorial](Getting-Started-First-Agent) - Build a real-world agent
- [Agent Development](Agent-Development) - Learn `.ai` file format
- [Configuration](Configuration) - Configure providers and tools
- [Headends](Headends) - Deploy as REST, MCP, Slack, etc.
