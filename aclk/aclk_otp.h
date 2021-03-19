// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_OTP_H
#define ACLK_OTP_H

#include "../daemon/common.h"

void aclk_get_mqtt_otp(RSA *p_key, char *aclk_hostname, int port, char **mqtt_usr, char **mqtt_pass);
int aclk_get_env(const char *aclk_hostname, int aclk_port);

#endif /* ACLK_OTP_H */
