#!/bin/sh

if [ -n "${CFLAGS}" ]; then
    echo "${CFLAGS}"
elif [ -n "${DEBUG_BUILD}" ]; then
    echo "-ffunction-sections -fdata-sections -Og -ggdb -pipe"
else
    echo "-ffunction-sections -fdata-sections -O2 -funroll-loops -pipe"
fi
