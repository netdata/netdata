# Getting Started

Get ai-agent installed and running your first agent in minutes.

---

## Table of Contents

- [What You'll Learn](#what-youll-learn) - Overview of this section
- [Minimum Requirements](#minimum-requirements) - System prerequisites
- [Quick Install](#quick-install) - Fastest path to installation
- [Verify Installation](#verify-installation) - Confirm ai-agent works
- [Your First Agent](#your-first-agent) - Minimal working example
- [Chapter Pages](#chapter-pages) - Detailed guides in this section
- [Quick Links](#quick-links) - Jump to common tasks
- [See Also](#see-also) - Related documentation

---

## What You'll Learn

This chapter covers everything you need to start using ai-agent:

- **Install ai-agent** on your system (multiple methods)
- **Configure** LLM providers (OpenAI, Anthropic, etc.)
- **Create and run** your first agent
- **Build a practical agent** with tools
- **Understand environment variables** for configuration

---

## Minimum Requirements

| Requirement | Minimum                     | Recommended    |
| ----------- | --------------------------- | -------------- |
| **Node.js** | 20+                         | 22 LTS         |
| **npm**     | 10+                         | Latest         |
| **OS**      | Linux, macOS, Windows (WSL) | Linux or macOS |
| **Memory**  | 512 MB                      | 1 GB+          |

> **Note:** Windows native support works but WSL is recommended for better compatibility with MCP servers.

---

## Quick Install

**Option A: Global installation (simplest)**

```bash
npm install -g @netdata/ai-agent
```

**Option B: From source (for development)**

```bash
git clone https://github.com/netdata/ai-agent.git
cd ai-agent
npm install
npm run build
npm link
```

For detailed installation options including local project installation, see [Installation](Getting-Started-Installation).

---

## Verify Installation

Run these commands to confirm ai-agent is installed correctly:

```bash
# Check version
ai-agent --version
```

**Expected output:**

```
ai-agent v0.0.0.698
```

```bash
# Check help
ai-agent --help
```

**Expected output:**

```
Usage: ai-agent [options] <system-prompt> <user-prompt>

AI Agent - Run intelligent agents powered by LLMs

Options:
  --agent <path>          Agent definition file (.ai)
  --config               Configuration file path
  --dry-run              Validate without calling LLM
  --verbose              Enable verbose logging
  -V, --version         Show version number
  -h, --help             Show this help message
  ...
```

---

## Your First Agent

Here's the fastest path to running an agent. Full details in [Quick Start](Getting-Started-Quick-Start).

**1. Set your API key:**

```bash
export OPENAI_API_KEY="sk-..."
```

**2. Create `.ai-agent.json`:**

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

**3. Create `hello.ai`:**

```yaml
#!/usr/bin/env ai-agent
---
description: A friendly greeting agent
models:
  - openai/gpt-4o-mini
maxTurns: 3
---
You are a friendly assistant. Greet the user and answer their questions.
```

**4. Run it:**

```bash
ai-agent --agent hello.ai "Hello, who are you?"
```

**Expected output:**

```
Hello! I'm a friendly AI assistant, here to help answer your questions
and have a pleasant conversation. How can I assist you today?
```

---

## Chapter Pages

| Page                                                           | Description                                       | Time      |
| -------------------------------------------------------------- | ------------------------------------------------- | --------- |
| [Installation](Getting-Started-Installation)                   | Prerequisites, installation methods, verification | 5 min     |
| [Quick Start](Getting-Started-Quick-Start)                     | Your first agent in 5 minutes                     | 5 min     |
| [First Agent Tutorial](Getting-Started-First-Agent)            | Build a real-world research agent with tools      | 15 min    |
| [Environment Variables](Getting-Started-Environment-Variables) | All environment configuration options             | Reference |

**Recommended path for new users:**

1. [Installation](Getting-Started-Installation) - Get ai-agent installed
2. [Quick Start](Getting-Started-Quick-Start) - Run your first simple agent
3. [First Agent Tutorial](Getting-Started-First-Agent) - Build something useful

---

## Quick Links

**Common tasks:**

- [How do I set up OpenAI?](Getting-Started-Quick-Start#step-2-create-a-configuration-file) - Quick Start step 2
- [How do I set up Anthropic?](Getting-Started-Quick-Start#step-2-create-a-configuration-file) - Quick Start step 2
- [What's a `.ai` file?](Getting-Started-Quick-Start#step-3-create-your-first-agent) - Quick Start step 3
- [How do I add tools?](Getting-Started-First-Agent#step-2-create-the-agent) - First Agent tutorial

**Troubleshooting:**

- ["No providers configured" error](Getting-Started-Quick-Start#common-issues)
- [API key errors](Getting-Started-Quick-Start#common-issues)
- [Model errors](Getting-Started-Quick-Start#common-issues)

---

## See Also

- [CLI Reference](CLI) - Complete command-line documentation
- [Agent Files](Agent-Files) - Full `.ai` file configuration reference
- [Configuration](Configuration) - Providers, MCP servers, and system settings
- [Headends](Headends) - Deploy agents as REST APIs, Slack bots, and more
- [Operations Troubleshooting](Operations-Troubleshooting) - Solutions to common problems
