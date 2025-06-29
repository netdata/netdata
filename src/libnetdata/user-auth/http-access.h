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

typedef enum __attribute__((packed)) {
    HTTP_ACCESS_NONE                        = 0,         //                                    adm man trb obs mem bil
    HTTP_ACCESS_SIGNED_ID                   = (1 << 0),  // User is authenticated               A   A   A   A   A   A
    HTTP_ACCESS_SAME_SPACE                  = (1 << 1),  // NC user+agent = same space          A   A   A   A   A   A
    HTTP_ACCESS_COMMERCIAL_SPACE            = (1 << 2),  // NC                                  P   P   P   P   P   P
    HTTP_ACCESS_ANONYMOUS_DATA              = (1 << 3),  // NC room:Read                        A   A   A   SR  SR  -
    HTTP_ACCESS_SENSITIVE_DATA              = (1 << 4),  // NC agent:ViewSensitiveData          A   A   A   -   SR  -
    HTTP_ACCESS_VIEW_AGENT_CONFIG           = (1 << 5),  // NC agent:ReadDynCfg                 P   P   -   -   -   -
    HTTP_ACCESS_EDIT_AGENT_CONFIG           = (1 << 6),  // NC agent:EditDynCfg                 P   P   -   -   -   -
    HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG   = (1 << 7),  // NC agent:ViewNotificationsConfig    P   -   -   -   -   -
    HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG   = (1 << 8),  // NC agent:EditNotificationsConfig    P   -   -   -   -   -
    HTTP_ACCESS_VIEW_ALERTS_SILENCING       = (1 << 9),  // NC space:GetSystemSilencingRules    A   A   A   -   A   -
    HTTP_ACCESS_EDIT_ALERTS_SILENCING       = (1 << 10), // NC space:CreateSystemSilencingRule  P   P   -   -   P   -
} HTTP_ACCESS;                                           //                                     ---------------------
                                                         //                                     A  = always
                                                         //                                     P  = commercial plan
                                                         //                                     SR = same room (Us+Ag)

#define HTTP_ACCESS_FORMAT "0x%" PRIx32
#define HTTP_ACCESS_FORMAT_CAST uint32_t

#define HTTP_ACCESS_ALL (HTTP_ACCESS)( \
      HTTP_ACCESS_SIGNED_ID \
    | HTTP_ACCESS_SAME_SPACE \
    | HTTP_ACCESS_COMMERCIAL_SPACE \
    | HTTP_ACCESS_ANONYMOUS_DATA \
    | HTTP_ACCESS_SENSITIVE_DATA \
    | HTTP_ACCESS_VIEW_AGENT_CONFIG \
    | HTTP_ACCESS_EDIT_AGENT_CONFIG \
    | HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG \
    | HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG \
    | HTTP_ACCESS_VIEW_ALERTS_SILENCING \
    | HTTP_ACCESS_EDIT_ALERTS_SILENCING \
)

#define HTTP_ACCESS_MAP_OLD_ANY (HTTP_ACCESS)(HTTP_ACCESS_ANONYMOUS_DATA)

#define HTTP_ACCESS_MAP_OLD_MEMBER (HTTP_ACCESS)( \
      HTTP_ACCESS_SIGNED_ID \
    | HTTP_ACCESS_SAME_SPACE \
    | HTTP_ACCESS_ANONYMOUS_DATA | HTTP_ACCESS_SENSITIVE_DATA)

#define HTTP_ACCESS_MAP_OLD_ADMIN (HTTP_ACCESS)( \
      HTTP_ACCESS_SIGNED_ID \
    | HTTP_ACCESS_SAME_SPACE \
    | HTTP_ACCESS_ANONYMOUS_DATA | HTTP_ACCESS_SENSITIVE_DATA | HTTP_ACCESS_VIEW_AGENT_CONFIG \
    | HTTP_ACCESS_EDIT_AGENT_CONFIG \
)

HTTP_ACCESS http_access2id_one(const char *str);
HTTP_ACCESS http_access2id(char *str);
struct web_buffer;
void http_access2buffer_json_array(struct web_buffer *wb, const char *key, HTTP_ACCESS access);
void http_access2txt(char *buf, size_t size, const char *separator, HTTP_ACCESS access);
HTTP_ACCESS http_access_from_hex(const char *str);
HTTP_ACCESS http_access_from_hex_mapping_old_roles(const char *str);
HTTP_ACCESS http_access_from_hex_str(const char *str);
HTTP_ACCESS http_access_from_source(const char *str);
bool log_cb_http_access_to_hex(struct web_buffer *wb, void *data);

#define HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(access) ((access & HTTP_ACCESS_SIGNED_ID) ? HTTP_RESP_FORBIDDEN : HTTP_RESP_PRECOND_FAIL)

typedef enum __attribute__((packed)) {
    HTTP_ACL_NONE                   = (0),

    HTTP_ACL_NOCHECK                = (1 << 0), // Don't check anything - adding this to an endpoint, disables ACL checking

    // transports
    HTTP_ACL_API                    = (1 << 1), // from the internal web server (TCP port)
    HTTP_ACL_API_UDP                = (1 << 2), // from the internal web server (UDP port)
    HTTP_ACL_API_UNIX               = (1 << 3), // from the internal web server (UNIX socket)
    HTTP_ACL_H2O                    = (1 << 4), // from the h2o web server
    HTTP_ACL_ACLK                   = (1 << 5), // from ACLK
    HTTP_ACL_WEBRTC                 = (1 << 6), // from WebRTC

    // HTTP_ACL_API takes the following additional ACLs, based on pattern matching of the client IP
    HTTP_ACL_METRICS                = (1 << 10),
    HTTP_ACL_FUNCTIONS              = (1 << 11),
    HTTP_ACL_NODES                  = (1 << 12),
    HTTP_ACL_ALERTS                 = (1 << 13),
    HTTP_ACL_DYNCFG                 = (1 << 14),
    HTTP_ACL_REGISTRY               = (1 << 15),
    HTTP_ACL_BADGES                 = (1 << 16),
    HTTP_ACL_MANAGEMENT             = (1 << 17),
    HTTP_ACL_STREAMING              = (1 << 18),
    HTTP_ACL_NETDATACONF            = (1 << 19),

    // SSL related
    HTTP_ACL_SSL_OPTIONAL           = (1 << 28),
    HTTP_ACL_SSL_FORCE              = (1 << 29),
    HTTP_ACL_SSL_DEFAULT            = (1 << 30),
} HTTP_ACL;

#define HTTP_ACL_DASHBOARD (HTTP_ACL)(                                  \
      HTTP_ACL_METRICS                                                  \
    | HTTP_ACL_FUNCTIONS                                                \
    | HTTP_ACL_ALERTS                                                   \
    | HTTP_ACL_NODES                                                    \
    | HTTP_ACL_DYNCFG                                                   \
 )

#define HTTP_ACL_TRANSPORTS (HTTP_ACL)(                                 \
      HTTP_ACL_API                                                      \
    | HTTP_ACL_API_UDP                                                  \
    | HTTP_ACL_API_UNIX                                                 \
    | HTTP_ACL_H2O                                                      \
    | HTTP_ACL_ACLK                                                     \
    | HTTP_ACL_WEBRTC                                                   \
)

#define HTTP_ACL_TRANSPORTS_WITHOUT_CLIENT_IP_VALIDATION (HTTP_ACL)(    \
      HTTP_ACL_ACLK                                                     \
    | HTTP_ACL_WEBRTC                                                   \
)

#define HTTP_ACL_ALL_FEATURES (HTTP_ACL)(                               \
      HTTP_ACL_METRICS                                                  \
    | HTTP_ACL_FUNCTIONS                                                \
    | HTTP_ACL_NODES                                                    \
    | HTTP_ACL_ALERTS                                                   \
    | HTTP_ACL_DYNCFG                                                   \
    | HTTP_ACL_REGISTRY                                                 \
    | HTTP_ACL_BADGES                                                   \
    | HTTP_ACL_MANAGEMENT                                               \
    | HTTP_ACL_STREAMING                                                \
    | HTTP_ACL_NETDATACONF                                              \
)

#define HTTP_ACL_ACLK_LICENSE_MANAGER (HTTP_ACL)(                       \
    HTTP_ACL_NODES                                                      \
)

#ifdef NETDATA_DEV_MODE
#define ACL_DEV_OPEN_ACCESS HTTP_ACL_NOCHECK
#else
#define ACL_DEV_OPEN_ACCESS 0
#endif

#define http_can_access_dashboard(w) (((w)->acl & HTTP_ACL_DASHBOARD) == HTTP_ACL_DASHBOARD)
#define http_can_access_registry(w) (((w)->acl & HTTP_ACL_REGISTRY) == HTTP_ACL_REGISTRY)
#define http_can_access_badges(w) (((w)->acl & HTTP_ACL_BADGES) == HTTP_ACL_BADGES)
#define http_can_access_mgmt(w) (((w)->acl & HTTP_ACL_MANAGEMENT) == HTTP_ACL_MANAGEMENT)
#define http_can_access_stream(w) (((w)->acl & HTTP_ACL_STREAMING) == HTTP_ACL_STREAMING)
#define http_can_access_netdataconf(w) (((w)->acl & HTTP_ACL_NETDATACONF) == HTTP_ACL_NETDATACONF)
#define http_is_using_ssl_optional(w) (((w)->port_acl & HTTP_ACL_SSL_OPTIONAL) == HTTP_ACL_SSL_OPTIONAL)
#define http_is_using_ssl_force(w) (((w)->port_acl & HTTP_ACL_SSL_FORCE) == HTTP_ACL_SSL_FORCE)
#define http_is_using_ssl_default(w) (((w)->port_acl & HTTP_ACL_SSL_DEFAULT) == HTTP_ACL_SSL_DEFAULT)

#endif //NETDATA_HTTP_ACCESS_H
