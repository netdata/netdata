// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MACOS_SMC_H
#define NETDATA_MACOS_SMC_H 1

#include "plugin_macos.h"

#include <IOKit/IOKitLib.h>
#include <stdint.h>

#define MACOS_SMC_KEY_LEN 4
#define MACOS_SMC_MAX_VALUE_SIZE 32

struct macos_smc_value {
    char key[MACOS_SMC_KEY_LEN + 1];
    char type[MACOS_SMC_KEY_LEN + 1];
    uint32_t size;
    uint8_t bytes[MACOS_SMC_MAX_VALUE_SIZE];
};

struct macos_smc_key_info {
    uint32_t data_size;
    uint32_t data_type;
    uint8_t data_attributes;
};

bool macos_smc_open(io_connect_t *connection);
void macos_smc_close(io_connect_t *connection);

bool macos_smc_key_count(io_connect_t connection, uint32_t *count);
bool macos_smc_key_by_index(io_connect_t connection, uint32_t index, char key[MACOS_SMC_KEY_LEN + 1]);
bool macos_smc_key_is_valid(const char key[MACOS_SMC_KEY_LEN + 1]);
bool macos_smc_read_key_info(
    io_connect_t connection,
    const char key[MACOS_SMC_KEY_LEN + 1],
    struct macos_smc_key_info *info);
bool macos_smc_read_key_with_info(
    io_connect_t connection,
    const char key[MACOS_SMC_KEY_LEN + 1],
    const struct macos_smc_key_info *info,
    struct macos_smc_value *value);
bool macos_smc_read_key(io_connect_t connection, const char key[MACOS_SMC_KEY_LEN + 1], struct macos_smc_value *value);

bool macos_smc_decode_numeric(const struct macos_smc_value *value, NETDATA_DOUBLE *decoded);
bool macos_smc_decode_temperature(const struct macos_smc_value *value, NETDATA_DOUBLE *decoded);

#endif /* NETDATA_MACOS_SMC_H */
