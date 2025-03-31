#!/bin/sh

# Exit immediately if a command exits with a non-zero status.
set -e

# Optional: Clean slate - remove existing module files
test -f "go.mod" && rm -f go.mod
test -f "go.sum" && rm -f go.sum

# 1. Initialize the Go module
echo "Initializing Go module..."
go mod init server

# 2. Tidy dependencies
echo "Tidying dependencies..."
go mod tidy

# 3. Build the main server executable using '.' for current package
echo "Building server executable..."
go build -o ./server .

# 4. Run the unit tests
echo "Running unit tests..."
go test -v

echo "Build and test script finished."
