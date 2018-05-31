#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0+

# will stop the script for any error
set -e

me="$0"
name="$1"
chart="$2"
conf="$3"

can_diff=1

tmp1="`mktemp`"
tmp2="`mktemp`"

myset() {
    set | grep -v "^_=" | grep -v "^PIPESTATUS=" | grep -v "^BASH_LINENO="
}

# save 2 'set'
myset >"$tmp1"
myset >"$tmp2"

# make sure they don't differ
diff "$tmp1" "$tmp2" >/dev/null 2>&1
if [ $? -ne 0 ]
then
    # they differ, we cannot do the check
    echo >&2 "$me: cannot check with diff."
    can_diff=0
fi

# do it again, now including the script
myset >"$tmp1"

# include the plugin and its config
if [ -f "$conf" ]
then
    . "$conf"
    if [ $? -ne 0 ]
    then
        echo >&2 "$me: cannot load config file $conf"
        rm "$tmp1" "$tmp2"
        exit 1
    fi
fi

. "$chart"
if [ $? -ne 0 ]
then
    echo >&2 "$me: cannot load chart file $chart"
    rm "$tmp1" "$tmp2"
    exit 1
fi

# remove all variables starting with the plugin name
myset | grep -v "^$name" >"$tmp2"

if [ $can_diff -eq 1 ]
then
    # check if they are different
    # make sure they don't differ
    diff "$tmp1" "$tmp2" >&2
    if [ $? -ne 0 ]
    then
        # they differ
        rm "$tmp1" "$tmp2"
        exit 1
    fi
fi

rm "$tmp1" "$tmp2"
exit 0
