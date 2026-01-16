# Installation

**Goal**: Install ai-agent on your system and verify it works.

---

## Table of Contents

- [Prerequisites](#prerequisites) - What you need before installing
- [Installation Methods](#installation-methods) - Choose how to install
- [Configuration Setup](#configuration-setup) - Create required config files
- [Environment Variables](#environment-variables) - Set up API keys
- [Verification](#verification) - Confirm everything works
- [Troubleshooting](#troubleshooting) - Fix common issues
- [Next Steps](#next-steps) - Where to go after installation
- [See Also](#see-also) - Related documentation

---

## Prerequisites

Before installing ai-agent, ensure you have:

| Requirement | Minimum Version | How to Check | Install Guide |
|-------------|-----------------|--------------|---------------|
| **Node.js** | 20+ | `node --version` | [nodejs.org](https://nodejs.org/) |
| **npm** | 10+ | `npm --version` | Comes with Node.js |
| **Git** | Any | `git --version` | [git-scm.com](https://git-scm.com/) (for source install only) |

**Check your versions:**

```bash
node --version && npm --version
```

**Expected output:**

```
v20.10.0
10.2.4
```

> **Note:** If Node.js is not installed or is an older version, download the LTS version from [nodejs.org](https://nodejs.org/).

---

## Installation Methods

### Global Installation (Recommended)

Install ai-agent globally for command-line use anywhere:

```bash
npm install -g @netdata/ai-agent
```

**Expected output:**

```
added 142 packages in 8s
```

**Verification:**

```bash
ai-agent --version
```

### Local Project Installation

Install as a project dependency:

```bash
cd your-project
npm install @netdata/ai-agent
```

Run via npx:

```bash
npx ai-agent --agent hello.ai "Hello"
```

Or add to `package.json` scripts:

```json
{
  "scripts": {
    "agent": "ai-agent"
  }
}
```

Then run:

```bash
npm run agent -- --agent hello.ai "Hello"
```

### From Source (For Development)

Clone and build from source:

```bash
# Clone the repository
git clone https://github.com/netdata/ai-agent.git
cd ai-agent

# Install dependencies
npm install

# Build
npm run build

# Link globally (optional)
npm link
```

**Expected output after `npm run build`:**

```
> ai-agent@1.0.0 build
> tsc

Build complete
```

**Verification:**

```bash
# If you ran npm link:
ai-agent --version

# Otherwise, run from the dist directory:
node dist/cli.js --version
```

---

## Configuration Setup

ai-agent requires a configuration file to connect to LLM providers. Create one of these files:

### Configuration File Locations

ai-agent searches for configuration in this order:

| Priority | Location | Use Case |
|----------|----------|----------|
| 1 | `--config <file>` CLI option | Override all defaults |
| 2 | `.ai-agent.json` in current directory | Project-specific config |
| 3 | `~/.ai-agent/ai-agent.json` | User-wide config |
| 4 | `/etc/ai-agent/ai-agent.json` | System-wide config |

### Minimal Configuration

Create `.ai-agent.json` in your working directory or `~/.ai-agent/ai-agent.json` for user-wide config:

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

**For multiple providers:**

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    },
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  }
}
```

> **Note:** The `${VAR}` syntax expands environment variables at runtime. Your API keys are never stored in the config file.

---

## Environment Variables

Store sensitive values like API keys in environment variables.

### Option 1: Export in Shell

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

Add to your shell profile (`~/.bashrc`, `~/.zshrc`) for persistence:

```bash
echo 'export OPENAI_API_KEY="sk-..."' >> ~/.bashrc
source ~/.bashrc
```

### Option 2: Environment File (Recommended)

Create `~/.ai-agent/ai-agent.env` or `.ai-agent.env` in your working directory:

```bash
# API Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# Optional: MCP server keys
BRAVE_API_KEY=...
GITHUB_TOKEN=ghp_...
```

ai-agent automatically loads this file on startup.

> **Security:** Add `.ai-agent.env` to your `.gitignore` to prevent committing secrets.

### Verify Environment Variables

```bash
# Check if variable is set
echo $OPENAI_API_KEY
```

**Expected output:**

```
sk-... (your key)
```

---

## Verification

Run these tests to confirm ai-agent is installed and configured correctly.

### Test 1: Version Check

```bash
ai-agent --version
```

**Expected:** Version number displayed (e.g., `ai-agent v1.0.0`)

### Test 2: Help Output

```bash
ai-agent --help
```

**Expected:** Help message with available options

### Test 3: Configuration Validation

Create a test agent file `test.ai`:

```yaml
#!/usr/bin/env ai-agent
---
description: Installation test agent
models:
  - openai/gpt-4o-mini
maxTurns: 1
---
Say "Installation successful!" and nothing else.
```

Run with `--dry-run` to validate without calling the LLM:

```bash
ai-agent --agent test.ai --dry-run "test"
```

**Expected output:**

```
[DRY-RUN] Configuration valid
[DRY-RUN] Agent file valid: test.ai
[DRY-RUN] Provider: openai
[DRY-RUN] Model: gpt-4o-mini
```

### Test 4: Live API Call

Run the test agent without `--dry-run`:

```bash
ai-agent --agent test.ai "test"
```

**Expected output:**

```
Installation successful!
```

---

## Troubleshooting

### "command not found: ai-agent"

**Cause:** Global npm binaries not in PATH.

**Solutions:**

1. Find npm global bin directory:
   ```bash
   npm config get prefix
   ```

2. Add to PATH (replace `/usr/local` with your prefix):
   ```bash
   export PATH="$PATH:/usr/local/bin"
   ```

3. Or use npx:
   ```bash
   npx ai-agent --version
   ```

### "No providers configured"

**Cause:** Configuration file not found.

**Solution:** Create `.ai-agent.json` in your current directory or `~/.ai-agent/ai-agent.json`. See [Configuration Setup](#configuration-setup).

### "API key not set" or "401 Unauthorized"

**Cause:** Environment variable not set or invalid key.

**Solutions:**

1. Verify the variable is exported:
   ```bash
   echo $OPENAI_API_KEY
   ```

2. If empty, export it:
   ```bash
   export OPENAI_API_KEY="sk-..."
   ```

3. Verify the key is valid in your provider's dashboard

### "ENOENT: no such file or directory"

**Cause:** Agent file not found.

**Solution:** Check the file path is correct:

```bash
ls -la test.ai  # Verify file exists
ai-agent --agent ./test.ai "test"  # Use explicit path
```

### npm install fails with permission errors

**Cause:** npm global directory needs elevated permissions.

**Solutions:**

1. Use a Node version manager (recommended):
   - [nvm](https://github.com/nvm-sh/nvm) for Linux/macOS
   - [nvm-windows](https://github.com/coreybutler/nvm-windows) for Windows

2. Or configure npm to use a user directory:
   ```bash
   mkdir ~/.npm-global
   npm config set prefix '~/.npm-global'
   export PATH="$PATH:~/.npm-global/bin"
   ```

---

## Next Steps

Installation complete. Continue with:

| Goal | Page |
|------|------|
| Run your first agent | [Quick Start](Getting-Started-Quick-Start) |
| Build a real-world agent | [First Agent Tutorial](Getting-Started-First-Agent) |
| Learn all environment options | [Environment Variables](Getting-Started-Environment-Variables) |
| Configure multiple providers | [Configuration Providers](Configuration-Providers) |

---

## See Also

- [Getting Started](Getting-Started) - Chapter overview
- [Quick Start](Getting-Started-Quick-Start) - Your first agent in 5 minutes
- [Configuration](Configuration) - Deep dive into configuration options
- [Configuration Files](Configuration-Files) - File resolution and layering
- [Operations Troubleshooting](Operations-Troubleshooting) - More troubleshooting solutions
