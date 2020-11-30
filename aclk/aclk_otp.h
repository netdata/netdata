// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_OTP_H
#define ACLK_OTP_H

#include "../daemon/common.h"

void aclk_get_mqtt_otp(RSA *p_key, char *aclk_hostname, int port, char **mqtt_usr, char **mqtt_pass);

#endif /* ACLK_OTP_H */
