#ifndef NETIPC_UDS_INTERNAL_H
#define NETIPC_UDS_INTERNAL_H

#include "netipc/netipc_protocol.h"
#include "netipc/netipc_uds.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <sys/types.h>

#define UDS_DEFAULT_BACKLOG 16
#define UDS_DEFAULT_BATCH_ITEMS 1

bool nipc_uds_header_payload_len(size_t payload_len, size_t *msg_len_out);
uint32_t nipc_uds_detect_packet_size(int fd);

nipc_uds_error_t nipc_uds_raw_send(int fd, const void *data, size_t len);
nipc_uds_error_t nipc_uds_raw_send_iov(int fd, const void *hdr, size_t hdr_len,
                                       const void *payload, size_t payload_len);
ssize_t nipc_uds_raw_recv(int fd, void *buf, size_t buf_len);

int nipc_uds_build_socket_name(char *dst, size_t dst_len,
                               const char *service_name);
int nipc_uds_build_socket_path(char *dst, size_t dst_len,
                               const char *run_dir,
                               const char *service_name);
bool nipc_uds_run_dir_allows_stale_unlink(const char *run_dir);
int nipc_uds_check_and_recover_stale(const char *run_dir,
                                     const char *socket_name,
                                     const char *path,
                                     bool allow_stale_unlink);

nipc_uds_error_t nipc_uds_client_handshake(int fd,
                                           const nipc_uds_client_config_t *cfg,
                                           nipc_uds_session_t *session);
nipc_uds_error_t nipc_uds_server_handshake(int fd,
                                           const nipc_uds_server_config_t *cfg,
                                           uint64_t session_id,
                                           nipc_uds_session_t *session);

int nipc_uds_inflight_add(nipc_uds_session_t *s, uint64_t id);
int nipc_uds_inflight_remove(nipc_uds_session_t *s, uint64_t id);
void nipc_uds_inflight_fail_all(nipc_uds_session_t *s);

#endif /* NETIPC_UDS_INTERNAL_H */
