#!/usr/bin/env bash

set -exu -o pipefail

clang++ \
    -std=c++11 -Wall -Wextra \
    -DENABLE_BENCHMARK -O2 -g \
    -lbenchmark -lbenchmark_main \
    -o gorilla_benchmark gorilla.cc

./gorilla_benchmark
