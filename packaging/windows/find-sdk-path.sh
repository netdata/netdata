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
  # vswhere.exe is the Microsoft-recommended way to locate VS installations,
  # guaranteed present on any machine with VS or VS Build Tools installed.
  local vswhere_path="/c/Program Files (x86)/Microsoft Visual Studio/Installer/vswhere.exe"

  if [ -f "$vswhere_path" ]; then
    echo "Checking for Visual Studio via vswhere.exe..." >&2
    local vs_install_path
    vs_install_path=$("$vswhere_path" -latest -products '*' \
      -requires 'Microsoft.VisualStudio.Component.VC.Tools.x86.x64' \
      -property installationPath 2>/dev/null | tr -d '\r\n')

    if [ -z "$vs_install_path" ]; then
      # Retry without component filter for stripped installs or Build Tools
      vs_install_path=$("$vswhere_path" -latest -products '*' \
        -property installationPath 2>/dev/null | tr -d '\r\n')
    fi

    if [ -n "$vs_install_path" ]; then
      local vs_unix_path msvc_tools_dir latest_msvc tool_path
      vs_unix_path=$(cygpath -u "$vs_install_path")
      msvc_tools_dir="$vs_unix_path/VC/Tools/MSVC"

      if [ -d "$msvc_tools_dir" ]; then
        latest_msvc=$(ls "$msvc_tools_dir" | grep -E "^[0-9]+\." | sort -V | tail -n 1)
        if [ -n "$latest_msvc" ]; then
          tool_path="$msvc_tools_dir/$latest_msvc/bin/Hostx64/x64"
          if [ -f "$tool_path/link.exe" ]; then
            echo "Found VS tools via vswhere in: \"$tool_path\"" >&2
            echo "$tool_path"
            return 0
          fi
        fi
      fi
      echo "WARNING: vswhere found VS at \"$vs_install_path\" but VC tools not available there." >&2
    else
      echo "WARNING: vswhere.exe found no VS installations with VC tools." >&2
    fi
  else
    echo "WARNING: vswhere.exe not found at \"$vswhere_path\", falling back to directory scan." >&2
  fi

  # Fall back: scan available year directories instead of using a fixed list
  local vs_base_path="/c/Program Files/Microsoft Visual Studio"
  if [ ! -d "$vs_base_path" ]; then
    echo "ERROR: Visual Studio base path \"$vs_base_path\" does not exist." >&2
    echo "$system_root"
    return 1
  fi

  local available_versions
  available_versions=$(ls "$vs_base_path" | grep -E "^[0-9]{4}$" | sort -V -r)
  if [ -z "$available_versions" ]; then
    echo "ERROR: No Visual Studio year directories found in \"$vs_base_path\"." >&2
    echo "$system_root"
    return 1
  fi

  echo "Found VS year directories: $(echo "$available_versions" | tr '\n' ' ')" >&2

  local found=0
  local tool_path_candidate
  for version in $available_versions; do
    if tool_path_candidate="$(check_visual_studio_version "${version}")"; then
      echo "${tool_path_candidate}"
      found=1
      break
    fi
  done

  if [ "${found}" -ne 1 ]; then
    echo "ERROR: Failed to find a usable version of Visual Studio" >&2
    echo "$system_root"
    return 1
  fi
}

check_visual_studio_version() {
  studio_base_path="/c/Program Files/Microsoft Visual Studio/${1}"
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
  if ! tool_path=$(find_sdk_tools); then
    exit 1
  fi
else
  if ! tool_path=$(find_visual_studio_tools); then
    exit 1
  fi
fi

# Output the discovered path
if [ "$windows_format" -eq 1 ]; then
  windows_tool_path=$(convert_to_windows_format "$tool_path")
  echo "$windows_tool_path"
else
  echo "$tool_path"
fi

exit 0
