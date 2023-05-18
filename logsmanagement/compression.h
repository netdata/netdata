// SPDX-License-Identifier: GPL-3.0-or-later

/** @file compression.h
 *  @brief Header of compression.c 
 */

#ifndef COMPRESSION_H_
#define COMPRESSION_H_

#include <lz4.h>
#include "circular_buffer.h"
#include "logsmanagement_conf.h"

int decompress_text(Circ_buff_item_t *const msg, char *const out_buf);

#endif  // COMPRESSION_H_
