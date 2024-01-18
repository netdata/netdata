// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#include "common_public.h"

// this dummy exists to have a special pointer with special meaning
// other than NULL
void _caller_responsibility(void *ptr) {
    (void)(ptr);
}
