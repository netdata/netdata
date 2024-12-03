#!/usr/bin/env bash

set -exu -o pipefail

CFLAGS="-Wall -Wextra -O2 -g -fsanitize=fuzzer,undefined,address"

re2c -b re2c_line_splitter.re.c -o re2c_line_splitter.c
clang $CFLAGS -c re2c_line_splitter.c -o re2c_line_splitter.o
clang++ $CFLAGS -c lines_splitter_pluginsd_fuzzer.cc -o lines_splitter_pluginsd_fuzzer.o
clang++ $CFLAGS re2c_line_splitter.o lines_splitter_pluginsd_fuzzer.o -o pluginsd_line_splitter_fuzzer

mkdir -p /tmp/corpus
echo 'simple word test' > /tmp/corpus/simple.txt
echo '"quoted string" test' > /tmp/corpus/quoted.txt
echo "'single quoted' test" > /tmp/corpus/single_quoted.txt
echo 'word1=word2 word3="quoted value"' > /tmp/corpus/mixed.txt
./pluginsd_line_splitter_fuzzer -workers=12 -jobs=16 /tmp/corpus
