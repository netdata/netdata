#!/bin/bash

mylocation=$(dirname "${0}")

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

# Determine paths for SDKs
if ! win_sdk_path="$("${mylocation}/../../../packaging/windows/find-sdk-path.sh" --sdk -w)"; then
    echo "ERROR: Failed to find Windows SDK"
    exit 1
fi
if ! vs_sdk_path="$("${mylocation}/../../../packaging/windows/find-sdk-path.sh" --visualstudio -w)"; then
    echo "ERROR: Failed to find Visual Studio SDK"
    exit 1
fi
# Write the contents to the temporary batch file
# Use cygpath directly within the heredoc
cat << EOF > "$temp_bat"
@echo off
set "PATH=%SystemRoot%\System32;${win_sdk_path};${vs_sdk_path}"
call "$(cygpath -w -a "$SCRIPT_DIR/wevt_netdata_compile.bat")" "$(cygpath -w -a "$src_dir")" "$(cygpath -w -a "$dest_dir")"
EOF

# Execute the temporary batch file
echo
echo "Executing Windows Batch File..."
echo
cat "$temp_bat"
cmd.exe //c "$(cygpath -w -a "$temp_bat")"
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
