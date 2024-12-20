// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SLEEP_H
#define NETDATA_SLEEP_H

void yield_the_processor(void);
void tinysleep(void);
void microsleep(usec_t ut);

#endif //NETDATA_SLEEP_H
