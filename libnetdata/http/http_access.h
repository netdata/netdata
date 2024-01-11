// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HTTP_ACCESS_H
#define NETDATA_HTTP_ACCESS_H

typedef enum __attribute__((packed)) {
    HTTP_ACCESS_NONE     = 0,
    HTTP_ACCESS_ADMIN    = 1,
    HTTP_ACCESS_MEMBER   = 2,
    HTTP_ACCESS_ANY      = 3,

    // keep this list so that lower numbers are more strict access levels
} HTTP_ACCESS;

const char *http_id2access(HTTP_ACCESS access);
HTTP_ACCESS http_access2id(const char *access);

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
