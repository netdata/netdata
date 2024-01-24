// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_ACCESS_H
#define NETDATA_HTTP_ACCESS_H

typedef enum __attribute__((packed)) {
    HTTP_USER_ROLE_NONE = 0,
    HTTP_USER_ROLE_ADMIN = 1,
    HTTP_USER_ROLE_MANAGER = 2,
    HTTP_USER_ROLE_TROUBLESHOOTER = 3,
    HTTP_USER_ROLE_OBSERVER = 4,
    HTTP_USER_ROLE_MEMBER = 5,
    HTTP_USER_ROLE_BILLING = 6,
    HTTP_USER_ROLE_ANY = 7,

    // keep this list so that lower numbers are more strict access levels
} HTTP_USER_ROLE;
const char *http_id2user_role(HTTP_USER_ROLE role);
HTTP_USER_ROLE http_user_role2id(const char *role);

typedef enum : uint32_t {
    HTTP_ACCESS_NONE                        = 0,
    HTTP_ACCESS_SIGNED_IN                   = (1 << 0),
    HTTP_ACCESS_CLAIM_AGENT                 = (1 << 1),
    HTTP_ACCESS_VIEW_ANONYMOUS_DATA         = (1 << 2),
    HTTP_ACCESS_VIEW_SENSITIVE_DATA         = (1 << 3),
    HTTP_ACCESS_VIEW_AGENT_CONFIG           = (1 << 4),
    HTTP_ACCESS_EDIT_AGENT_CONFIG           = (1 << 5),
    HTTP_ACCESS_VIEW_COLLECTION_CONFIG      = (1 << 6),
    HTTP_ACCESS_EDIT_COLLECTION_CONFIG      = (1 << 7),
    HTTP_ACCESS_VIEW_ALERTS_CONFIG          = (1 << 8),
    HTTP_ACCESS_EDIT_ALERTS_CONFIG          = (1 << 9),
    HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG   = (1 << 10),
    HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG   = (1 << 11),
    HTTP_ACCESS_VIEW_ALERTS_SILENCING       = (1 << 12),
    HTTP_ACCESS_EDIT_ALERTS_SILENCING       = (1 << 13),
    HTTP_ACCESS_VIEW_STREAMING_CONFIG       = (1 << 14),
    HTTP_ACCESS_EDIT_STREAMING_CONFIG       = (1 << 15),
    HTTP_ACCESS_VIEW_EXPORTING_CONFIG       = (1 << 16),
    HTTP_ACCESS_EDIT_EXPORTING_CONFIG       = (1 << 17),
} HTTP_ACCESS;

#define HTTP_ACCESS_ACLK_DEFAULT                                                                                       \
    (HTTP_ACCESS_SIGNED_IN|HTTP_ACCESS_VIEW_ANONYMOUS_DATA|HTTP_ACCESS_VIEW_SENSITIVE_DATA)

#define HTTP_ACCESS_ALL ((HTTP_ACCESS)(0xFFFFFFFF))

HTTP_ACCESS http_access2id_one(const char *str);
HTTP_ACCESS http_access2id(char *str);
struct web_buffer;
void http_access2buffer_json_array(struct web_buffer *wb, const char *key, HTTP_ACCESS access);
void http_access2txt(char *buf, size_t size, char separator, HTTP_ACCESS access);
HTTP_ACCESS https_access_from_base64_bitmap(const char *str);
HTTP_ACCESS http_access_from_hex(const char *str);
HTTP_ACCESS http_access_from_source(const char *str);
bool log_cb_http_access_to_hex(struct web_buffer *wb, void *data);

typedef enum __attribute__((packed)) {
    HTTP_ACL_NONE                   = (0),
    HTTP_ACL_NOCHECK                = (1 << 0),          // Don't check anything - this should work on all channels
    HTTP_ACL_DASHBOARD              = (1 << 1),
    HTTP_ACL_REGISTRY               = (1 << 2),
    HTTP_ACL_BADGE                  = (1 << 3),
    HTTP_ACL_MGMT                   = (1 << 4),
    HTTP_ACL_STREAMING              = (1 << 5),
    HTTP_ACL_NETDATACONF            = (1 << 6),
    HTTP_ACL_SSL_OPTIONAL           = (1 << 7),
    HTTP_ACL_SSL_FORCE              = (1 << 8),
    HTTP_ACL_SSL_DEFAULT            = (1 << 9),
    HTTP_ACL_ACLK                   = (1 << 10),
    HTTP_ACL_WEBRTC                 = (1 << 11),
    HTTP_ACL_BEARER_IF_PROTECTED    = (1 << 12), // allow unprotected access if bearer is not enabled in netdata
    HTTP_ACL_BEARER_REQUIRED        = (1 << 13), // allow access only if a valid bearer is used
    HTTP_ACL_BEARER_OPTIONAL        = (1 << 14), // the call may or may not need a bearer - will be determined later
} HTTP_ACL;

#define HTTP_ACL_DASHBOARD_ACLK_WEBRTC (HTTP_ACL_DASHBOARD | HTTP_ACL_ACLK | HTTP_ACL_WEBRTC | HTTP_ACL_BEARER_IF_PROTECTED)
#define HTTP_ACL_ACLK_WEBRTC_DASHBOARD_WITH_OPTIONAL_BEARER (HTTP_ACL_DASHBOARD | HTTP_ACL_ACLK | HTTP_ACL_WEBRTC | HTTP_ACL_BEARER_OPTIONAL)

#ifdef NETDATA_DEV_MODE
#define ACL_DEV_OPEN_ACCESS HTTP_ACL_NOCHECK
#else
#define ACL_DEV_OPEN_ACCESS 0
#endif

#define http_can_access_dashboard(w) ((w)->acl & HTTP_ACL_DASHBOARD)
#define http_can_access_registry(w) ((w)->acl & HTTP_ACL_REGISTRY)
#define http_can_access_badges(w) ((w)->acl & HTTP_ACL_BADGE)
#define http_can_access_mgmt(w) ((w)->acl & HTTP_ACL_MGMT)
#define http_can_access_stream(w) ((w)->acl & HTTP_ACL_STREAMING)
#define http_can_access_netdataconf(w) ((w)->acl & HTTP_ACL_NETDATACONF)
#define http_is_using_ssl_optional(w) ((w)->port_acl & HTTP_ACL_SSL_OPTIONAL)
#define http_is_using_ssl_force(w) ((w)->port_acl & HTTP_ACL_SSL_FORCE)
#define http_is_using_ssl_default(w) ((w)->port_acl & HTTP_ACL_SSL_DEFAULT)

#endif //NETDATA_HTTP_ACCESS_H
