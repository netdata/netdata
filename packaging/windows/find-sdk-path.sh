#!/bin/bash

# Function to output the path in Windows format (convert from MSYS2/Unix format using cygpath)
convert_to_windows_format() {
  cygpath -w -a "$1"
}

# Function to display help message
display_help() {
  echo "Usage: $0 [-s|--sdk] [-v|--visualstudio] [-w|--windows] [--help]"
  echo
  echo "Options:"
  echo "  -s, --sdk            Search for tools in the Windows SDK."
  echo "  -v, --visualstudio   Search for tools in Visual Studio."
  echo "  -w, --windows        Output the path in Windows format (using cygpath)."
  echo "  --help               Display this help message."
  exit 0
}

# Function to find tools in the Windows SDK
find_sdk_tools() {
  sdk_base_path="/c/Program Files (x86)/Windows Kits/10/bin"

  if [ ! -d "$sdk_base_path" ]; then
    echo "ERROR: SDK base path \"$sdk_base_path\" does not exist. No SDK installations found." >&2
    echo "$system_root"
    return 1
  fi

  echo "SDK base path exists: \"$sdk_base_path\"" >&2

  # Find all SDK versions
  sdk_versions=($(ls "$sdk_base_path" | tr ' ' '\n' | grep -E "^[0-9]+\..*$"))
  echo "Found SDK versions: ${sdk_versions[*]}" >&2

  if [ ${#sdk_versions[@]} -eq 0 ]; then
    echo "ERROR: No valid Windows SDK versions found in \"$sdk_base_path\"." >&2
    echo "$system_root"
    return 1
  fi

  # Sort versions and pick the latest
  sorted_versions=$(printf '%s\n' "${sdk_versions[@]}" | sort -V)
  latest_sdk_version=$(echo "$sorted_versions" | tail -n 1)
  sdk_tool_path="$sdk_base_path/$latest_sdk_version/x64"

  echo "Latest SDK version: \"$latest_sdk_version\"" >&2

  if [ ! -d "$sdk_tool_path" ]; then
    echo "ERROR: Tool path \"$sdk_tool_path\" does not exist." >&2
    echo "$system_root"
    return 1
  fi

  # Check if required tools exist
  tools=("mc.exe" "rc.exe")
  for tool in "${tools[@]}"; do
    if [ ! -f "$sdk_tool_path/$tool" ]; then
      echo "ERROR: $tool not found in \"$sdk_tool_path\"" >&2
      echo "$system_root"
      return 1
    else
      echo "$tool found in \"$sdk_tool_path\"" >&2
    fi
  done

  echo >&2
  echo "DONE: All required tools found in \"$sdk_tool_path\"" >&2
  echo >&2

  echo "$sdk_tool_path"
}

# Function to find tools in Visual Studio
find_visual_studio_tools() {
  studio_base_path="/c/Program Files/Microsoft Visual Studio/2022"
  echo "Checking for Visual Studio installations in: \"$studio_base_path\"" >&2

  if [ ! -d "$studio_base_path" ]; then
    echo "ERROR: Visual Studio base path \"$studio_base_path\" does not exist. No Visual Studio installations found." >&2
    echo "$system_root"
    return 1
  fi

  # Visual Studio editions we want to check
  editions=("Enterprise" "Professional" "Community")
  available_editions=()

  # Loop through each edition and check for tools
  for edition in "${editions[@]}"; do
    edition_path="$studio_base_path/$edition/VC/Tools/MSVC"
    if [ -d "$edition_path" ]; then
      available_editions+=("$edition")
      echo "Checking edition: $edition in $studio_base_path" >&2

      # Find all MSVC versions and sort them
      msvc_versions=($(ls "$edition_path" | tr ' ' '\n' | grep -E "^[0-9]+\..*$"))
      echo "Found MSVC versions in $edition: ${msvc_versions[*]}" >&2

      if [ ${#msvc_versions[@]} -gt 0 ]; then
        sorted_versions=$(printf '%s\n' "${msvc_versions[@]}" | sort -V)
        latest_msvc_version=$(echo "${sorted_versions[@]}" | tail -n 1)
        vs_tool_path="$edition_path/$latest_msvc_version/bin/Hostx64/x64"

        echo "Latest MSVC version: \"$latest_msvc_version\" in $edition" >&2

        if [ ! -d "$vs_tool_path" ]; then
          echo "WARNING: Tool path \"$vs_tool_path\" does not exist." >&2
          continue
        fi

        # Check if required tools exist
        tools=("link.exe")
        missing_tool=0

        for tool in "${tools[@]}"; do
          if [ ! -f "$vs_tool_path/$tool" ]; then
            echo "WARNING: $tool not found in \"$vs_tool_path\" for $edition" >&2
            missing_tool=1
          else
            echo "$tool found in \"$vs_tool_path\"" >&2
          fi
        done

        if [ $missing_tool -eq 0 ]; then
          echo >&2
          echo "All required tools found in \"$vs_tool_path\"" >&2
          echo >&2

          echo "$vs_tool_path"
          return 0
        else
          echo "WARNING: skipping edition '$edition', directory does not exist." >&2
        fi
      else
        echo "WARNING: skipping edition '$edition', MSVC directory does not exist." >&2
      fi
    else
      echo "WARNING: skipping edition '$edition', directory does not exist." >&2
    fi
  done

  echo "ERROR: No valid Visual Studio editions found in \"$studio_base_path\"." >&2
  echo "$system_root"
  return 1
}

# Parse options using getopt
TEMP=$(getopt -o svwh --long sdk,visualstudio,windows,help -- "$@")
if [ $? != 0 ]; then
  echo "ERROR: Invalid options provided." >&2
  exit 1
fi

eval set -- "$TEMP"

search_mode="sdk"
windows_format=0
system_root="/usr/bin"

# Process getopt options
while true; do
  case "$1" in
    -s|--sdk)
        search_mode="sdk"
        shift
        ;;
    -v|--visualstudio)
        search_mode="visualstudio"
        shift
        ;;
    -w|--windows)
        system_root="%SYSTEMROOT%"
        windows_format=1
        shift
        ;;
    --help|-h)
        display_help
        ;;
    --)
        shift
        break
        ;;
    *)
        echo "ERROR: Invalid option: $1" >&2
        exit 1
        ;;
  esac
done

# Ensure that one of --sdk or --visualstudio is selected
if [ -z "$search_mode" ]; then
  echo "ERROR: You must specify either --sdk or --visualstudio." >&2
  display_help
fi

# Determine which function to call based on the search mode
if [ "$search_mode" = "sdk" ]; then
  tool_path=$(find_sdk_tools)
else
  tool_path=$(find_visual_studio_tools)
fi

# If a valid path is found, output it
if [ "$tool_path" != "$system_root" ]; then
  if [ "$windows_format" -eq 1 ]; then
    windows_tool_path=$(convert_to_windows_format "$tool_path")
    echo "$windows_tool_path"
  else
    echo "$tool_path"
  fi
else
  echo "$system_root"
  exit 1
fi

exit 0
