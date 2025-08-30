#!/bin/sh

if [ -n "${CFLAGS}" ]; then
    echo "${CFLAGS}"
elif [ -n "${DEBUG_BUILD}" ]; then
    echo "-Og -ggdb -pipe"
else
    echo "-O2 -funroll-loops -pipe"
fi
