// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_USER_AUTH_H
#define NETDATA_USER_AUTH_H

#include "../libnetdata.h"
#include "../template-enum.h"

#define CLOUD_CLIENT_NAME_LENGTH 64

typedef enum auth_type {
    USER_AUTH_METHOD_NONE = 0,
    USER_AUTH_METHOD_CLOUD,
    USER_AUTH_METHOD_BEARER,
    USER_AUTH_METHOD_GOD,
} USER_AUTH_METHOD;

typedef struct user_auth {
    char client_ip[INET6_ADDRSTRLEN];
    char forwarded_for[INET6_ADDRSTRLEN];
    char client_name[CLOUD_CLIENT_NAME_LENGTH];
    ND_UUID cloud_account_id;
    USER_AUTH_METHOD method;
    HTTP_USER_ROLE user_role;
    HTTP_ACCESS access;
} USER_AUTH;

// Declare the enum-to-string conversion functions
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(USER_AUTH_METHOD)

bool user_auth_from_source(const char *src, USER_AUTH *parsed);

bool user_auth_source_is_cloud(const char *source);

void user_auth_to_source_buffer(USER_AUTH *user_auth, BUFFER *source);

#endif //NETDATA_USER_AUTH_H
