// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CLOUD_CMDS_H
#define ACLK_SCHEMA_WRAPPER_CLOUD_CMDS_H

#ifdef __cplusplus
extern "C" {
#endif

struct disconnect_cmd {
    uint64_t reconnect_after_s;
    int permaban;
    uint32_t error_code;
    char *error_desctiprion;
};

struct disconnect_cmd *parse_disconnect_cmd(const char *data, size_t len);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER__CLOUD_CMDS_H */
