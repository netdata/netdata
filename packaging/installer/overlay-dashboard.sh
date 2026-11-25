#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Merge CMake's freshly installed files into an existing web tree. Only create
# missing directories: existing directories and their link targets keep their modes.
set -eu
unset CDPATH

merge_directory() (
    staged=$1
    release=$2
    cd -P "$3"
    destination=$PWD
    cd -P "$staged"
    set --
    for file in ./* ./.[!.]* ./..?*; do
        [ -e "$file" ] || [ -L "$file" ] || continue
        name=${file#./}
        if [ -d "$file" ] && [ ! -L "$file" ]; then
            [ -d "$destination/$name" ] || mkdir -m 755 "$destination/$name"
            merge_directory "$staged/$name" "$release/$name" "$destination/$name"
            continue
        fi
        if [ -d "$destination/$name" ] && { [ ! -L "$destination/$name" ] || [ ! -L "$release/$name" ]; }; then
            echo >&2 "Cannot install dashboard file over directory: $destination/$name"
            exit 1
        fi
        # CMake's relative links point back to the persistent release tree.
        # Recompute them from the physical install directory after relocation.
        if [ -L "$file" ] && [ ! -L "$release/$name" ]; then
            case "$(readlink "$file")" in
                /* | ?:/*) target="$release/$name" ;;
                *)
                    parent=$destination
                    up=''
                    while :; do
                        case "$release/$name" in "${parent%/}/"*) break ;; esac
                        parent=${parent%/*}
                        up="../$up"
                    done
                    path="$release/$name"
                    target="$up${path#"${parent%/}/"}"
                    ;;
            esac
            if ln -sf "$target" "$destination/$name"; then continue; fi
            case ${CMAKE_INSTALL_MODE:-} in
                *_OR_COPY)
                    rm -f "$file"
                    cp -p "$release/$name" "$file"
                    chmod 644 "$file"
                    ;;
                *) exit 1 ;;
            esac
        fi
        set -- "$@" "$file"
        if [ "$#" -eq 128 ]; then
            mv -f "$@" "$destination"
            set --
        fi
    done
    if [ "$#" -gt 0 ]; then mv -f "$@" "$destination"; fi
)

cd -P "$2"
merge_directory "$1" "$PWD" "$3"
