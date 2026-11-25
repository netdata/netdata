// SPDX-License-Identifier: GPL-3.0-or-later

// Standalone runner for the fopen_secret_write() checks.
//
// The checks themselves live in fopen_secret_write_unittest() next to the
// implementation in src/libnetdata/paths/paths.c, so that `netdata -W unittest`
// runs them in CI through paths_unittest(). This target only exists to run them
// on their own, without building the daemon:
//
//   cmake --build build --target fopen-secret-write-test && ./build/fopen-secret-write-test

#include "libnetdata/libnetdata.h"

int main(void) {
    return fopen_secret_write_unittest() ? 1 : 0;
}
