// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OTEL_H
#define NETDATA_OTEL_H

#ifdef __cplusplus
extern "C" {
#endif

void otel_init(void);
void otel_fini(void);

void *otel_main(void *ptr);

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_OTEL_H */
