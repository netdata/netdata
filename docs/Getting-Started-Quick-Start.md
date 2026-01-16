# Quick Start

**Goal**: Create and run your first AI agent in under 5 minutes.

---

## Table of Contents

- [Prerequisites](#prerequisites) - What you need before starting
- [Step 1: Set Up Your API Key](#step-1-set-up-your-api-key) - Configure LLM provider access
- [Step 2: Create a Configuration File](#step-2-create-a-configuration-file) - Set up ai-agent.json
- [Step 3: Create Your First Agent](#step-3-create-your-first-agent) - Write a .ai file
- [Step 4: Run Your Agent](#step-4-run-your-agent) - Execute and see output
- [Step 5: Verify It Worked](#step-5-verify-it-worked) - Test different scenarios
- [Common Issues](#common-issues) - Troubleshooting guide
- [What You Learned](#what-you-learned) - Summary of skills gained
- [Next Steps](#next-steps) - Where to go from here

---

## Prerequisites

Before starting, ensure you have:

- **ai-agent installed** - See [Installation](Getting-Started-Installation)
- **At least one LLM provider API key** - OpenAI, Anthropic, or another supported provider
- **A terminal/command line** - Any Unix shell or Windows PowerShell

---

## Step 1: Set Up Your API Key

Export your API key as an environment variable. Choose the provider you have access to:

**OpenAI:**
```bash
export OPENAI_API_KEY="sk-..."
```

**Anthropic:**
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**Verification**: Confirm the variable is set:
```bash
echo $OPENAI_API_KEY
# Should print: sk-... (your key, partially hidden for security)
```

> **Tip:** For persistent configuration, store keys in `~/.ai-agent/ai-agent.env` instead of exporting. See [Environment Variables](Getting-Started-Environment-Variables).

---

## Step 2: Create a Configuration File

Create a configuration file in your working directory. This tells ai-agent how to connect to your LLM provider.

Create `.ai-agent.json`:

**For OpenAI:**
```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

**For Anthropic:**
```json
{
  "providers": {
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  }
}
```

> **Note:** The `${VAR}` syntax expands environment variables at runtime. Your actual API key never appears in the config file.

---

## Step 3: Create Your First Agent

Create a file named `hello.ai` with this content:

**For OpenAI:**
```yaml
#!/usr/bin/env ai-agent
---
description: A friendly greeting agent
models:
  - openai/gpt-4o-mini
maxTurns: 3
---
You are a friendly assistant. Greet the user warmly and answer their questions concisely.
```

**For Anthropic:**
```yaml
#!/usr/bin/env ai-agent
---
description: A friendly greeting agent
models:
  - anthropic/claude-sonnet-4-20250514
maxTurns: 3
---
You are a friendly assistant. Greet the user warmly and answer their questions concisely.
```

**File structure explained:**

| Section | Purpose |
|---------|---------|
| `#!/usr/bin/env ai-agent` | Shebang - makes the file executable |
| `---` | YAML frontmatter delimiters |
| `description` | Human-readable agent description |
| `models` | LLM model(s) to use (format: `provider/model`) |
| `maxTurns` | Maximum conversation turns before completing |
| Text after `---` | System prompt - instructions for the agent |

---

## Step 4: Run Your Agent

Execute the agent with a user prompt:

```bash
ai-agent --agent hello.ai "Hello, who are you?"
```

**Expected output:**
```
Hello! I'm a friendly AI assistant. I'm here to help you with questions, provide
information, or just have a pleasant conversation. How can I assist you today?
```

> **Note:** The exact wording varies with each run since LLMs are non-deterministic. The response should be friendly and conversational.

---

## Step 5: Verify It Worked

Confirm your agent is working correctly by testing these scenarios:

**Test 1: Different question**
```bash
ai-agent --agent hello.ai "What can you help me with?"
```
Expected: A response listing capabilities or offering assistance.

**Test 2: Dry run (validation only)**
```bash
ai-agent --agent hello.ai --dry-run "Test"
```
Expected: Validates configuration and agent file without calling the LLM.

**Test 3: Verbose mode (for debugging)**
```bash
ai-agent --agent hello.ai --verbose "Hello"
```
Expected: Additional logging showing configuration resolution, model selection, and timing.

---

## Common Issues

### "No providers configured"

**Cause:** Configuration file not found or empty.

**Solution:** Ensure `.ai-agent.json` exists in the current directory or `~/.ai-agent/ai-agent.json` in your home directory.

### "API key not set" or authentication errors

**Cause:** Environment variable not exported or incorrect key.

**Solution:**
1. Verify the variable is set: `echo $OPENAI_API_KEY`
2. Re-export if needed: `export OPENAI_API_KEY="sk-..."`
3. Check the key is valid in your provider's dashboard

### "Unknown model" or model errors

**Cause:** Model name doesn't match provider's available models.

**Solution:** Use exact model names. For example:
- OpenAI: `openai/gpt-4o`, `openai/gpt-4o-mini`, `openai/gpt-4-turbo`
- Anthropic: `anthropic/claude-sonnet-4-20250514`, `anthropic/claude-3-5-sonnet-20241022`

### Agent file syntax errors

**Cause:** Invalid YAML in frontmatter.

**Solution:**
1. Ensure exactly three dashes (`---`) on separate lines
2. Check indentation (use 2 spaces, not tabs)
3. Ensure quotes around values with special characters

---

## What You Learned

After completing this quick start, you can:

- Configure ai-agent to connect to an LLM provider
- Create a basic `.ai` agent file with frontmatter
- Run an agent from the command line
- Use `--dry-run` and `--verbose` for debugging

---

## Next Steps

Now that your first agent works, continue learning:

| Goal | Page |
|------|------|
| Build a real-world agent with tools | [First Agent Tutorial](Getting-Started-First-Agent) |
| Understand the `.ai` file format | [Agent Files](Agent-Development-Agent-Files) |
| Learn all frontmatter options | [Frontmatter Reference](Agent-Development-Frontmatter) |
| Deploy agents as REST APIs | [REST Headend](Headends-REST) |
| Complete CLI options | [CLI Reference](Getting-Started-CLI-Reference) |

---

## See Also

- [Installation](Getting-Started-Installation) - Detailed installation options
- [Environment Variables](Getting-Started-Environment-Variables) - All environment configuration
- [Configuration](Configuration) - Deep dive into configuration files
- [Troubleshooting](Operations-Troubleshooting) - Solutions to common problems
