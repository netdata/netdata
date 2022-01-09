// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_OTP_H
#define ACLK_OTP_H

#include "daemon/common.h"

#include "https_client.h"
#include "aclk_util.h"

int aclk_get_mqtt_otp(RSA *p_key, char **mqtt_id, char **mqtt_usr, char **mqtt_pass, url_t *target);
int aclk_get_env(aclk_env_t *env, const char *aclk_hostname, int aclk_port);

#endif /* ACLK_OTP_H */
