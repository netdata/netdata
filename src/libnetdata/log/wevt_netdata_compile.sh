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

# Write the contents to the temporary batch file
# Use cygpath directly within the heredoc
cat << EOF > "$temp_bat"
@echo off
set "PATH=%SYSTEMROOT%;$("${mylocation}/../../../packaging/windows/find-sdk-path.sh" --sdk -w);$("${mylocation}/../../../packaging/windows/find-sdk-path.sh" --visualstudio -w)"
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
