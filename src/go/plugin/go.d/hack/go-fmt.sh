#!/bin/sh

# SPDX-License-Identifier: GPL-3.0-or-later

for TARGET in "${@}"; do
  find "${TARGET}" -name '*.go' -exec gofmt -s -w {} \+
done
git diff --exit-code
