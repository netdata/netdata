// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef GORILLA_H
#define GORILLA_H

#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gorilla_page_writer gpw_t;

gpw_t *gpw_new();
void gpw_free(gpw_t *gpw);

void gpw_add_buffer(gpw_t *gpw);
bool gpw_add(gpw_t *gw, uint32_t value);
void gpw_flush(gpw_t *gw);

#ifdef __cplusplus
}
#endif

#endif /* GORILLA_H */
