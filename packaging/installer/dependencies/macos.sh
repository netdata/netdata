#!/usr/bin/env zsh
# SPDX-License-Identifier: GPL-3.0-or-later
########################################################################################################
#Script Name    : install-required-packages.sh
#Args           :
#Required files :
#OS             : macOS Catalina or newer
#Description    : Install required packages if Homebrew is installed
#Author         : Igor Karpov
#Email          : igor@netdata.cloud
#
#To do          : Whistles and bells
#
########################################################################################################
zmodload zsh/zutil
set -euf -o localoptions -o localtraps

interactive_mode=true

TRAPINT() {
    print "Caught SIGINT, aborting."
    return $(( 128 +$1 ))
}

err() {
  echo "[$(date +'h%Y-%m-%dT%H:%M:%S%z')]: $*" >&2
}

usage() {
    echo "Usage: install-required-packages.sh [-d] [-n] [-a] [-h]"
    echo “Try 'install-required-packages.sh -h' for more information.”
    exit 1
}

help() {
    cat <<EOF
install-required-packages.sh [-d|--dont-wait] [-n|--non-interactive] [-a|--all] [-h]

This script is intended to be run on macOS with Homebrew installed.
Its purpose is to install the packages needed to install or run netdata.
It is predominantly called by kickstart.sh, but can be called manually
independently.
If run without arguments, the script will run in interactive mode, asking for
confirmation for each of the following actions:
    brew update
    brew upgrade
    brew install <required packages>

Possible arguments are:
-d | --dont-wait                Same as '-n' Obsoleted, kept for backward compatibility only.
-n | --non-interactive          Ask no questions.
-a | --all                      The true meaning of this important option to be revealed later. ;)
-h                              This help.
EOF
    exit(0)
}

homebrew_update() {
    brew update
}

homebrew_upgrade() {
    brew upgrade $(brew outdated --cask --greedy --quiet)
}

homebrew_install() {
    brew install openssl
    brew install lz4
    brew install libuv
}

zparseopts -D -a option \
    'n' '-non-interactive' \
    'd' '-dont-wait' \
    'h' '-help'

case ${option[1]} in
    -n | --non-interactive)
    interactive_mode=false
    ;;
    -d | --dont-wait)
    interactive_mode=false
    ;;
    -h | --help)
    ;;
esac

main() {

    if command -v brew > /dev/null 2>&1; then
        echo "we need brew... found"
    else
        err "we need Homebrew... please install it before running this script"
        exit 1
    fi

    if [ "$interactive_mode" = true ]
    then
        read "consent_given?Ok to run brew update? (y/n) "
            if [[ "$consent_given" =~ ^[Yy]$ ]]             # Consent given
            then
                homebrew_update
            fi
    else                                                    # Non-interactive mode, no consent needed
        homebrew_update
    fi

    if [ "$interactive_mode" = true ]
    then
        read "consent_given?Ok to run brew upgrade? (y/n) "
            if [[ "$consent_given" =~ ^[Yy]$ ]]             # Consent given
            then
                homebrew_upgrade
            fi
    else
        homebrew_upgrade                                    # Non-interactive mode, no consent needed
    fi

    if [ "$interactive_mode" = true ]
    then
        read "consent_given?Ok to install the required packages with Homebrew? (y/n) "
            if [[ "$consent_given" =~ ^[Yy]$ ]]             # Consent given
            then
                homebrew_install
            fi
    else                                                    # Non-interactive mode, no consent needed
        homebrew_install
    fi
}

main
