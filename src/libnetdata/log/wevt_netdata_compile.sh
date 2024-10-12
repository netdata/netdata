#!/bin/bash

# Check if both parameters are provided
if [ $# -ne 2 ]; then
    echo "Error: Incorrect number of parameters."
    echo "Usage: $0 <source_directory> <destination_directory>"
    exit 1
fi

# Get the parameters
src_dir="$1"
dest_dir="$2"

# Get the directory of this script
SCRIPT_DIR="$(dirname "$0")"

# Create a temporary batch file
temp_bat=$(mktemp --suffix=.bat)

# Write the contents to the temporary batch file
# Use cygpath directly within the heredoc
cat << EOF > "$temp_bat"
@echo off
call "$(cygpath -w -a "$SCRIPT_DIR/wevt_netdata_compile.bat")" "$(cygpath -w -a "$src_dir")" "$(cygpath -w -a "$dest_dir")"
EOF

# show the batch file
cat "$temp_bat"

WTEMP_BAT="$(cygpath -w -a "$temp_bat")"

# Filter out any paths that refer to MSYS and only keep paths starting with /c/
# because link.exe exists also in msys...
OLD_PATH="${PATH}"
PATH="$(echo "$PATH" | tr ':' '\n' | grep '^/c/' | tr '\n' ';'):/c/windows/system32"

# Execute the temporary batch file
echo
echo "Executing Windows Batch File..."
cmd.exe //c "${WTEMP_BAT}"

# Capture the exit status
exit_status=$?

PATH="${OLD_PATH}"

# Remove the temporary batch file
rm "$temp_bat"

# Check the exit status
if [ $exit_status -eq 0 ]; then
    echo "nd_wevents_compile.bat executed successfully."
else
    echo "nd_wevents_compile.bat failed with exit status $exit_status."
fi

exit $exit_status
