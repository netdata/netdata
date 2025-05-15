# Netdata MCP Bridge - Python Implementation

This Python bridge converts MCP stdio communication to Netdata's MCP over WebSocket.

## Requirements

- Python 3.7+
- websockets library

## Installation

The easiest way to set up the Python bridge is to use the included build script, which creates a virtual environment (recommended for most Linux distributions where pip is restricted to virtual environments):

```bash
# Make the build script executable (if needed)
chmod +x build.sh

# Run the build script
./build.sh
```

The script will:
1. Check if Python 3 is installed
2. Create a virtual environment in the `venv` directory if it doesn't exist
3. Install the required websockets dependency in the virtual environment
4. Generate a requirements.txt file for reproducibility

### Running with Virtual Environment

After running the build script:

```bash
# Activate the virtual environment
source venv/bin/activate

# Run the bridge
python nd-mcp.py ws://localhost:19999/mcp

# When finished, deactivate the virtual environment
deactivate
```

Or run it directly with the virtual environment's Python:

```bash
./venv/bin/python nd-mcp.py ws://localhost:19999/mcp
```

### Manual Installation (without virtual environment)

If your system allows global pip installations:

```bash
pip install websockets
```

## Usage

The script can be run directly as an executable (the shebang line will use the system's Python):

```bash
./nd-mcp.py ws://<ip>:19999/mcp
```

Or explicitly with Python:

```bash
python nd-mcp.py ws://<ip>:19999/mcp
```

When using a virtual environment, run it with the environment's Python:

```bash
./venv/bin/python nd-mcp.py ws://<ip>:19999/mcp
```

Where `<ip>` is either `localhost` or the IP address where a Netdata instance is listening.

## Example with Claude Desktop

To use this bridge with Claude Desktop:

1. In Claude Desktop settings, configure the Custom Command option:

```bash
python /path/to/stdio-python/nd-mcp.py ws://localhost:19999/mcp
```

2. If your Netdata instance is running on a different machine, replace `localhost` with the appropriate IP address.

## How It Works

The bridge:
1. Establishes a WebSocket connection to the specified Netdata MCP endpoint
2. Reads from standard input and sends to the WebSocket
3. Receives messages from the WebSocket and writes to standard output
4. Handles both directions simultaneously

## Protocol Compatibility

- Netdata MCP implements the JSON-RPC 2.0 protocol
- Messages that don't conform to the JSON-RPC 2.0 format are silently ignored
- The bridge passes messages directly without any modification