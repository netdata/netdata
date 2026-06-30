// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef STRNDUP_H
#define STRNDUP_H

#include "config.h"

#include <stddef.h>

#ifndef HAVE_STRNDUP
char *strndup(const char *s, size_t n);
#endif

#endif //STRNDUP_H
