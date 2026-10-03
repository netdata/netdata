#!/bin/bash

set -eu -o pipefail

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
    echo "Usage: $0 <executable> <destination-directory> [dependency-manifest]" >&2
    exit 1
fi

executable="$1"
destination="$2"
runtime_dll_dir="${NETDATA_WINDOWS_RUNTIME_DLL_DIR:-/ucrt64/bin}"
dependency_manifest="${3:-}"
resolved_dlls=()
windows_system_root="${SYSTEMROOT:-${SystemRoot:-}}"
_netdata_windows_system32=""
if [ -n "${windows_system_root}" ] && command -v cygpath >/dev/null 2>&1; then
    if system_root_msys="$(cygpath -u -- "${windows_system_root}")"; then
        _netdata_windows_system32="${system_root_msys%/}/system32"
        _netdata_windows_system32="${_netdata_windows_system32,,}"
    fi
fi

if [ -n "${dependency_manifest}" ] && { [ -e "${dependency_manifest}" ] || [ -L "${dependency_manifest}" ]; }; then
    if [ ! -f "${dependency_manifest}" ]; then
        echo "ERROR: dependency manifest is not a regular file: ${dependency_manifest}" >&2
        exit 1
    fi
    if [ ! -r "${dependency_manifest}" ]; then
        echo "ERROR: dependency manifest is not readable: ${dependency_manifest}" >&2
        exit 1
    fi
    while IFS= read -r dll; do
        [ -n "${dll}" ] && resolved_dlls+=("${dll}")
    done < "${dependency_manifest}"
fi

if [ ! -f "${executable}" ] || [ ! -r "${executable}" ]; then
    echo "ERROR: executable not found: ${executable}" >&2
    exit 1
fi

if [ ! -d "${runtime_dll_dir}" ]; then
    echo "ERROR: UCRT64 runtime DLL directory not found: ${runtime_dll_dir}" >&2
    exit 1
fi

if ! command -v ldd.exe >/dev/null 2>&1; then
    echo "ERROR: ldd.exe not found in PATH; cannot stage Windows runtime DLLs." >&2
    exit 1
fi

mkdir -p "${destination}"

for stale_system_dll in \
    CRYPT32.dll WLDAP32.dll SHELL32.dll bcrypt.dll Secur32.dll USERENV.dll \
    SSPICLI.DLL CRYPTBASE.DLL dbghelp.dll dbgcore.DLL; do
    rm -f "${destination}/${stale_system_dll}"
done

is_ignored_dll() {
    local dll="${1,,}"

    case "${dll}" in
        api-ms-*|ext-ms-*|advapi32.dll|bcrypt.dll|cfgmgr32.dll|combase.dll|crypt32.dll|cryptbase.dll|\
        dbgcore.dll|dbghelp.dll|gdi32.dll|imm32.dll|iphlpapi.dll|kernel32.dll|kernelbase.dll|\
        msvcrt.dll|ntdll.dll|ole32.dll|oleaut32.dll|powrprof.dll|psapi.dll|rpcrt4.dll|\
        secur32.dll|setupapi.dll|shell32.dll|shlwapi.dll|user32.dll|userenv.dll|version.dll|\
        wer.dll|werfault.dll|winhttp.dll|winmm.dll|winspool.drv|ws2_32.dll|wldap32.dll)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

is_msys_dll() {
    case "${1,,}" in
        msys-*.dll|msys2-*.dll)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

copy_missing_dlls_once() {
    local copied=0
    local unresolved=()
    local dll
    local source
    local resolved_path
    local normalized_resolved_path

    local dependency_output
    if ! dependency_output="$(PATH="${destination}:${runtime_dll_dir}:${PATH}" ldd.exe "${executable}" 2>&1)"; then
        echo "ERROR: ldd.exe failed while inspecting ${executable}: ${dependency_output:-no diagnostic}" >&2
        return 3
    fi
    if [ -z "${dependency_output}" ]; then
        echo "ERROR: ldd.exe returned no dependency information for ${executable}" >&2
        return 3
    fi

    while IFS= read -r line; do
        case "${line}" in
            *"=>"*)
                dll="${line%%=>*}"
                dll="${dll#"${dll%%[![:space:]]*}"}"
                dll="${dll%"${dll##*[![:space:]]}"}"
                dll="${dll##*/}"
                resolved_path="${line#*=>}"
                resolved_path="${resolved_path#"${resolved_path%%[![:space:]]*}"}"
                resolved_path="${resolved_path%"${resolved_path##*[![:space:]]}"}"
                normalized_resolved_path="${resolved_path,,}"
                normalized_resolved_path="${normalized_resolved_path//\\//}"
                if is_msys_dll "${dll}"; then
                    unresolved+=("${dll} (MSYS runtime dependency)")
                    continue
                fi

                if [ -n "${_netdata_windows_system32:-}" ] &&
                   [[ "${normalized_resolved_path}" == "${_netdata_windows_system32}"/* ]]; then
                        continue
                fi

                if is_ignored_dll "${dll}"; then
                    continue
                fi

                source="${runtime_dll_dir}/${dll}"

                if [ ! -f "${source}" ]; then
                    unresolved+=("${dll}")
                    continue
                fi

                local seen=false
                local resolved
                for resolved in "${resolved_dlls[@]}"; do
                    if [ "${resolved,,}" = "${dll,,}" ]; then
                        seen=true
                        break
                    fi
                done
                if [ "${seen}" = false ]; then
                    resolved_dlls+=("${dll}")
                fi

                # Refresh managed copies after runtime updates, but avoid copying a
                # file onto itself when the runtime directory is the destination.
                if [ ! -f "${destination}/${dll}" ] ||
                    { [ ! "${source}" -ef "${destination}/${dll}" ] &&
                      ! cmp -s "${source}" "${destination}/${dll}"; }; then
                    cp -f "${source}" "${destination}/${dll}"
                    copied=$((copied + 1))
                    echo "Staged Windows runtime DLL: ${dll}"
                fi
                ;;
        esac
    done <<< "${dependency_output}"

    if [ "${#unresolved[@]}" -gt 0 ]; then
        printf 'ERROR: unresolved non-system Windows runtime DLLs for %s:\n' "${executable}" >&2
        printf '  %s\n' "${unresolved[@]}" >&2
        return 2
    fi

    if [ "${copied}" -eq 0 ]; then
        return 0
    fi

    return 1
}

for _ in $(seq 1 20); do
    if copy_missing_dlls_once; then
        if [ -n "${dependency_manifest}" ]; then
            printf '%s\n' "${resolved_dlls[@]}" > "${dependency_manifest}"
        fi
        exit 0
    else
        rc=$?
    fi

    if [ "${rc}" -ne 1 ]; then
        exit "${rc}"
    fi
done

echo "ERROR: runtime DLL staging did not converge for ${executable}" >&2
exit 1
