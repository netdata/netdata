#!/usr/bin/env bash
#
# SPDX-License-Identifier: GPL-3.0-or-later
#

set -exu -o pipefail

clang++ \
    -std=c++11 -Wall -Wextra \
    -DENABLE_FUZZER -O2 -g \
    -fsanitize=fuzzer \
    -o gorilla_fuzzer gorilla.cc

./gorilla_fuzzer -workers=12 -jobs=16
