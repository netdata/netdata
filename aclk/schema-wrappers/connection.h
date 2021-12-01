// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CONNECTION_H
#define ACLK_SCHEMA_WRAPPER_CONNECTION_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    const char *claim_id;
    unsigned int reachable:1;

    int64_t session_id;

    unsigned int lwt:1;

// TODO in future optional fields
// > 15 optional fields:
// How long the system was running until connection (only applicable when reachable=true)
//    google.protobuf.Duration system_uptime = 15;
// How long the netdata agent was running until connection (only applicable when reachable=true)
//    google.protobuf.Duration agent_uptime = 16;


} update_agent_connection_t;

char *generate_update_agent_connection(size_t *len, const update_agent_connection_t *data);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CONNECTION_H */
