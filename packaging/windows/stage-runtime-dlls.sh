#!/bin/bash

set -eu -o pipefail

if [ $# -ne 2 ]; then
    echo "Usage: $0 <executable> <destination-directory>" >&2
    exit 1
fi

executable="$1"
destination="$2"
runtime_dll_dir="${NETDATA_WINDOWS_RUNTIME_DLL_DIR:-/ucrt64/bin}"

if [ ! -f "${executable}" ]; then
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
        lib*.dll|zlib1.dll)
            return 1
            ;;
        api-ms-*|ext-ms-*)
            return 0
            ;;
        *)
            return 0
            ;;
    esac
}

copy_missing_dlls_once() {
    local copied=0
    local unresolved=()
    local dll
    local resolved
    local source

    while IFS= read -r line; do
        case "${line}" in
            *"=>"*)
                dll="${line%%=>*}"
                dll="${dll#"${dll%%[![:space:]]*}"}"
                dll="${dll%"${dll##*[![:space:]]}"}"
                dll="${dll##*/}"
                resolved="${line#*=>}"
                resolved="${resolved%%(*}"
                resolved="${resolved#"${resolved%%[![:space:]]*}"}"
                resolved="${resolved%"${resolved##*[![:space:]]}"}"

                if is_ignored_dll "${dll}"; then
                    continue
                fi

                source="${runtime_dll_dir}/${dll}"
                if [ ! -f "${source}" ] && [ -f "${resolved}" ]; then
                    source="${resolved}"
                fi

                if [ ! -f "${source}" ]; then
                    if [ "${resolved}" = "not found" ]; then
                        unresolved+=("${dll}")
                    fi
                    continue
                fi

                if [ ! -f "${destination}/${dll}" ]; then
                    cp "${source}" "${destination}/${dll}"
                    copied=$((copied + 1))
                    echo "Staged Windows runtime DLL: ${dll}"
                fi
                ;;
        esac
    done < <(PATH="${destination}:${runtime_dll_dir}:${PATH}" ldd.exe "${executable}" 2>/dev/null || true)

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
        exit 0
    fi

    rc=$?
    if [ "${rc}" -eq 2 ]; then
        exit 2
    fi
done

echo "ERROR: runtime DLL staging did not converge for ${executable}" >&2
exit 1
