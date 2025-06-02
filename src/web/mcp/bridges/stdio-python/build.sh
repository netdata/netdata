#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Working directory: $SCRIPT_DIR"

# Check if Python is installed
if ! command -v python3 &> /dev/null; then
    echo "Error: Python 3 is not installed or not in the PATH"
    echo "Please install Python 3 from https://www.python.org/ or through your package manager"
    exit 1
fi

echo "Python is installed: $(python3 --version)"

# Create virtual environment if it doesn't exist
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
    echo "Virtual environment created at $SCRIPT_DIR/venv"
else
    echo "Virtual environment already exists at $SCRIPT_DIR/venv"
fi

# Activate virtual environment
echo "Activating virtual environment..."
source venv/bin/activate || { echo "Failed to activate virtual environment"; exit 1; }

# Check pip version
echo "pip version: $(pip --version)"

# Install requirements
echo "Installing dependencies..."
pip install websockets

# Create requirements.txt if it doesn't exist
if [ ! -f "requirements.txt" ]; then
    echo "Creating requirements.txt..."
    pip freeze > requirements.txt
    echo "requirements.txt created"
else
    echo "requirements.txt already exists"
    echo "To update it, run: source $SCRIPT_DIR/venv/bin/activate && pip freeze > requirements.txt"
fi

# Get the full path to the script and venv
SCRIPT_PATH="$SCRIPT_DIR/nd-mcp.py"
VENV_PYTHON="$SCRIPT_DIR/venv/bin/python"
VENV_ACTIVATE="$SCRIPT_DIR/venv/bin/activate"

# Make sure the script is executable
chmod +x "$SCRIPT_PATH"

echo "Build complete!"
echo ""
echo "To use the bridge with the virtual environment:"
echo ""
echo "Option 1: Activate the environment and run the script"
echo "source $VENV_ACTIVATE"
echo "python $SCRIPT_PATH ws://ip:19999/mcp"
echo "deactivate  # when finished"
echo ""
echo "Option 2: Run directly with the virtual environment's Python"
echo "$VENV_PYTHON $SCRIPT_PATH ws://ip:19999/mcp"
echo ""
echo "Option 3: Run the script directly (using the system's Python)"
echo "$SCRIPT_PATH ws://ip:19999/mcp"