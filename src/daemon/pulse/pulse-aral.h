// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_ARAL_H
#define NETDATA_PULSE_ARAL_H

#include "daemon/common.h"

void pulse_aral_register_statistics(struct aral_statistics *stats, const char *name);
void pulse_aral_unregister_statistics(struct aral_statistics *stats);

void pulse_aral_register(ARAL *ar, const char *name);
void pulse_aral_unregister(ARAL *ar);

#if defined(PULSE_INTERNALS)
void pulse_aral_init(void);
void pulse_aral_do(bool extended);
#endif

#endif //NETDATA_PULSE_ARAL_H
