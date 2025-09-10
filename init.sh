#!/usr/bin/env bash

set -e  # Exit on error

if [ ! -d mcp/mcp-gsc ]; then
	echo "Setting up mcp-gsc..."
	
	# Create mcp directory if it doesn't exist
	mkdir -p mcp
	cd mcp
	
	# Clone the repository
	echo "Cloning mcp-gsc repository..."
	git clone https://github.com/AminForou/mcp-gsc mcp-gsc
	cd mcp-gsc
	
	# Create virtual environment
	echo "Creating virtual environment..."
	python3 -m venv .venv
	
	# Activate virtual environment
	echo "Activating virtual environment..."
	source .venv/bin/activate
	
	# Ensure pip is up to date
	echo "Updating pip..."
	python3 -m pip install --upgrade pip
	
	# Install dependencies
	echo "Installing requirements..."
	python3 -m pip install -r requirements.txt
	
	echo "mcp-gsc setup complete!"
	cd ../..
else
	echo "mcp-gsc already exists in mcp/mcp-gsc"
fi