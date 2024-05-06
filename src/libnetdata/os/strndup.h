// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef STRNDUP_H
#define STRNDUP_H

#include "config.h"

#ifndef HAVE_STRNDUP
#define strndup(s, n) os_strndup(s, n)
#endif

#endif //STRNDUP_H
