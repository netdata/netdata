#!/usr/bin/env bash

find . -type f -name '*.[ch]' | while read filename; do echo -ne "$filename: "; diff <(clang-format -style=file $filename) $filename | wc -l; done
