#!/usr/bin/env bash

find . -type f -name '*.[ch]' | while read -r filename; do echo -ne "$filename: "; diff <(clang-format -style=file "$filename") "$filename" | wc -l; done
