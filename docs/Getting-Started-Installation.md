# Installation

## Prerequisites

- **Node.js 20+** - Required runtime
- **npm 10+** - Package manager
- **Git** - For cloning the repository

---

## Installation Methods

### From Source (Recommended for Development)

```bash
# Clone the repository
git clone https://github.com/netdata/ai-agent.git
cd ai-agent

# Install dependencies
npm install

# Build
npm run build

# Optional: Install globally
npm link
```

### Global Installation

```bash
npm install -g @netdata/ai-agent
```

### Local Project Installation

```bash
npm install @netdata/ai-agent
```

---

## Configuration File

AI Agent requires a configuration file. Create `.ai-agent.json` in your working directory:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {},
  "defaults": {
    "llmTimeout": 120000,
    "toolTimeout": 60000,
    "temperature": 0.7
  }
}
```

### Configuration Resolution Order

1. `--config <filename>` CLI option
2. `.ai-agent.json` in current directory
3. `~/.ai-agent.json` in home directory
4. System-wide `/etc/ai-agent/.ai-agent.json`

---

## Environment Variables

Store sensitive values in `.ai-agent.env` (loaded automatically):

```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

Or export them in your shell:

```bash
export OPENAI_API_KEY="sk-..."
```

---

## Verify Installation

```bash
# Check version
ai-agent --version

# Check help
ai-agent --help

# Dry run (validates config without calling LLM)
ai-agent --agent my-agent.ai --dry-run
```

---

## Next Steps

- [Quick Start](Getting-Started-Quick-Start) - Run your first agent
- [Configuration](Configuration) - Deep dive into configuration options
