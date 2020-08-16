#!/bin/bash

KPROBE_FILE="/sys/kernel/debug/tracing/kprobe_events"

DATA="$(grep _netdata_ $KPROBE_FILE| cut -d' ' -f1 | cut -d: -f2)"

for I in $DATA; do
    echo "-:$I" > $KPROBE_FILE 2>/dev/null;
done
