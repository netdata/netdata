#!/bin/bash

# Check if both parameters are provided
if [ $# -ne 2 ]; then
    echo "Error: Incorrect number of parameters."
    echo "Usage: $0 <path_to_mc_file> <destination_directory>"
    exit 1
fi

# Get the parameters
mc_file="$1"
dest_dir="$2"

# Get the directory of this script
SCRIPT_DIR="$(dirname "$0")"

# Create a temporary batch file
temp_bat=$(mktemp --suffix=.bat)

# Write the contents to the temporary batch file
# Use cygpath directly within the heredoc
cat << EOF > "$temp_bat"
@echo off
call "$(cygpath -w -a "$SCRIPT_DIR/nd_wevents_compile.bat")" "$(cygpath -w -a "$mc_file")" "$(cygpath -w -a "$dest_dir")"
EOF

grep call <"$temp_bat"

# Execute the temporary batch file
cmd.exe //c "$(cygpath -w -a "$temp_bat")"

# Capture the exit status
exit_status=$?

# Remove the temporary batch file
rm "$temp_bat"

# Check the exit status
if [ $exit_status -eq 0 ]; then
    echo "nd_wevents_compile.bat executed successfully."
else
    echo "nd_wevents_compile.bat failed with exit status $exit_status."
fi

exit $exit_status