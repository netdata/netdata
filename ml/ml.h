// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_H
#define NETDATA_ML_H

#ifdef __cplusplus
extern "C" {
#endif

#include "daemon/common.h"

#if defined(ENABLE_ML_TESTS)
int test_ml(int argc, char *argv[]);
#endif

#ifdef __cplusplus
};
#endif

#endif /* NETDATA_ML_H */
