// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_GENERATE_H
#define NETDATA_UUID_GENERATE_H

void os_uuid_generate(void *out);
void os_uuid_generate_random(void *out);
void os_uuid_generate_time(void *out);

#endif //NETDATA_UUID_GENERATE_H
