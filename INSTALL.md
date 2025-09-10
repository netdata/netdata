# AI Agent Installation Guide

## Overview

This guide describes how to install AI Agent from source to a production environment at `/opt/ai-agent`.

## Architecture

### Source Repository (Development)
```
~/src/ai-agent/
├── src/              # TypeScript source code
├── dist/             # Compiled JavaScript (git-ignored)
├── node_modules/     # Dependencies (git-ignored)
├── neda/             # Agent templates and configs
├── mcp/              # MCP server sources
├── docs/             # Documentation
├── package.json      # Node.js configuration
├── tsconfig.json     # TypeScript configuration
└── build-and-install.sh  # Installation script
```

### Production Installation
```
/opt/ai-agent/
├── dist/             # Compiled application
├── node_modules/     # Production dependencies
├── neda/             # Agent runtime
│   ├── *.ai          # Agent scripts
│   ├── .ai-agent.json     # Configuration (overwritten on update)
│   ├── .ai-agent.env      # User secrets (preserved)
│   └── .gsc-client-secrets.json  # Google secrets (preserved)
├── mcp/              # MCP servers
├── cache/            # Runtime caches
├── tmp/              # Temporary files
├── logs/             # Application logs
└── ai-agent          # Launcher script
```

## Installation Steps

### 1. Prerequisites

- Node.js 18+ and npm
- Python 3.8+ (for MCP servers)
- sudo access for system installation

### 2. Build and Install

Run the installation script:

```bash
sudo ./build-and-install.sh
```

This script will:
1. Build the TypeScript application
2. Create the `ai-agent` system user (if needed)
3. Install to `/opt/ai-agent`
4. Preserve existing user configurations
5. Set up MCP servers
6. Configure systemd service
7. Create `/usr/local/bin/ai-agent` symlink

### 3. Configuration

#### Environment Variables
Edit `/opt/ai-agent/neda/.ai-agent.env`:
```bash
sudo -u ai-agent nano /opt/ai-agent/neda/.ai-agent.env
```

Add your API keys and configuration.

#### Google Credentials
Add your Google service account credentials:
```bash
sudo -u ai-agent nano /opt/ai-agent/neda/.gsc-client-secrets.json
```

### 4. Service Management

Start the service:
```bash
sudo systemctl start neda
```

Enable auto-start on boot:
```bash
sudo systemctl enable neda
```

Check status:
```bash
sudo systemctl status neda
```

View logs:
```bash
journalctl -u neda -f
```

Restart after configuration changes:
```bash
sudo systemctl restart neda
```

## Updates

To update the installation:

1. Pull latest changes:
```bash
cd ~/src/ai-agent
git pull
```

2. Run the installer again:
```bash
sudo ./build-and-install.sh
```

The installer will:
- Backup the current installation
- Preserve user configurations (`.ai-agent.env`, `.gsc-client-secrets.json`)
- Update application code and `.ai-agent.json`

## Rollback

If an update causes issues, backups are created automatically:

```bash
# List backups
ls -la /opt/ai-agent.backup.*

# Restore a backup (example)
sudo systemctl stop neda
sudo mv /opt/ai-agent /opt/ai-agent.broken
sudo mv /opt/ai-agent.backup.20240110_143022 /opt/ai-agent
sudo systemctl start neda
```

## Security Notes

- The service runs as the `ai-agent` system user (non-privileged)
- Sensitive files (`.ai-agent.env`, `.gsc-client-secrets.json`) have 600 permissions
- The service uses systemd's security features (NoNewPrivileges, PrivateTmp)

## Troubleshooting

### Service won't start
```bash
# Check logs
journalctl -u neda -n 50

# Check file permissions
ls -la /opt/ai-agent/neda/

# Verify user exists
id ai-agent
```

### Permission errors
```bash
# Fix ownership
sudo chown -R ai-agent:ai-agent /opt/ai-agent

# Fix permissions on secrets
sudo chmod 600 /opt/ai-agent/neda/.ai-agent.env
sudo chmod 600 /opt/ai-agent/neda/.gsc-client-secrets.json
```

### Command not found
```bash
# Verify symlink
ls -la /usr/local/bin/ai-agent

# Recreate if needed
sudo ln -sf /opt/ai-agent/ai-agent /usr/local/bin/ai-agent
```

## Development vs Production

- **Development**: Run from source with `npm run start` in `~/src/ai-agent`
- **Production**: Run installed version with `ai-agent` command or via systemd service

The development environment remains independent, allowing you to test changes before deploying to production.