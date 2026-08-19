#include "netipc_uds_internal.h"

#include <string.h>

static inline uint32_t min_u32(uint32_t a, uint32_t b) { return a < b ? a : b; }

static inline uint32_t apply_default(uint32_t val, uint32_t def) {
  return val == 0 ? def : val;
}

static uint32_t highest_bit(uint32_t mask) {
  if (mask == 0)
    return 0;

  uint32_t bit = 1u << 31;
  while (!(mask & bit))
    bit >>= 1;
  return bit;
}

static bool header_version_incompatible(const void *buf, size_t buf_len,
                                        uint16_t expected_code) {
  if (buf_len < NIPC_HEADER_LEN)
    return false;

  nipc_header_t hdr;
  memcpy(&hdr, buf, sizeof(hdr));
  return hdr.magic == NIPC_MAGIC_MSG && hdr.version != NIPC_VERSION &&
         hdr.header_len == NIPC_HEADER_LEN && hdr.kind == NIPC_KIND_CONTROL &&
         hdr.code == expected_code;
}

static bool hello_layout_incompatible(const void *buf, size_t buf_len) {
  if (buf_len < sizeof(uint16_t))
    return false;

  nipc_hello_t hello;
  memset(&hello, 0, sizeof(hello));
  memcpy(&hello, buf, sizeof(uint16_t));
  return hello.layout_version != 1;
}

static bool hello_ack_layout_incompatible(const void *buf, size_t buf_len) {
  if (buf_len < sizeof(uint16_t))
    return false;

  nipc_hello_ack_t ack;
  memset(&ack, 0, sizeof(ack));
  memcpy(&ack, buf, sizeof(uint16_t));
  return ack.layout_version != 1;
}

static void encode_control_header(nipc_header_t *hdr, uint16_t code,
                                  uint16_t status, uint32_t payload_len) {
  *hdr = (nipc_header_t){
      .magic = NIPC_MAGIC_MSG,
      .version = NIPC_VERSION,
      .header_len = NIPC_HEADER_LEN,
      .kind = NIPC_KIND_CONTROL,
      .code = code,
      .transport_status = status,
      .payload_len = payload_len,
      .item_count = 1,
  };
}

static void send_rejection_ack(int fd, uint16_t status) {
  nipc_hello_ack_t ack = {.layout_version = 1};
  uint8_t ack_buf[48];
  uint8_t pkt[80];
  nipc_header_t ack_hdr;

  encode_control_header(&ack_hdr, NIPC_CODE_HELLO_ACK, status, sizeof(ack_buf));
  nipc_hello_ack_encode(&ack, ack_buf, sizeof(ack_buf));
  nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
  memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));
  nipc_uds_raw_send(fd, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
}

static nipc_uds_error_t ack_status_to_error(uint16_t status) {
  if (status == NIPC_STATUS_OK)
    return NIPC_UDS_OK;
  if (status == NIPC_STATUS_AUTH_FAILED)
    return NIPC_UDS_ERR_AUTH_FAILED;
  if (status == NIPC_STATUS_UNSUPPORTED)
    return NIPC_UDS_ERR_NO_PROFILE;
  if (status == NIPC_STATUS_INCOMPATIBLE)
    return NIPC_UDS_ERR_INCOMPATIBLE;
  if (status == NIPC_STATUS_LIMIT_EXCEEDED)
    return NIPC_UDS_ERR_LIMIT_EXCEEDED;
  return NIPC_UDS_ERR_HANDSHAKE;
}

static void fill_client_session(nipc_uds_session_t *session, int fd,
                                const nipc_hello_ack_t *ack) {
  session->fd = fd;
  session->role = NIPC_UDS_ROLE_CLIENT;
  session->max_request_payload_bytes = ack->agreed_max_request_payload_bytes;
  session->max_request_batch_items = ack->agreed_max_request_batch_items;
  session->max_response_payload_bytes = ack->agreed_max_response_payload_bytes;
  session->max_response_batch_items = ack->agreed_max_response_batch_items;
  session->packet_size = ack->agreed_packet_size;
  session->selected_profile = ack->selected_profile;
  session->session_id = ack->session_id;
  session->recv_buf = NULL;
  session->recv_buf_size = 0;
}

static void fill_server_session(nipc_uds_session_t *session, int fd,
                                uint32_t selected,
                                const nipc_hello_ack_t *ack) {
  session->fd = fd;
  session->role = NIPC_UDS_ROLE_SERVER;
  session->max_request_payload_bytes = ack->agreed_max_request_payload_bytes;
  session->max_request_batch_items = ack->agreed_max_request_batch_items;
  session->max_response_payload_bytes = ack->agreed_max_response_payload_bytes;
  session->max_response_batch_items = ack->agreed_max_response_batch_items;
  session->packet_size = ack->agreed_packet_size;
  session->selected_profile = selected;
  session->session_id = ack->session_id;
  session->recv_buf = NULL;
  session->recv_buf_size = 0;
}

nipc_uds_error_t nipc_uds_client_handshake(int fd,
                                           const nipc_uds_client_config_t *cfg,
                                           nipc_uds_session_t *session) {
  uint8_t buf[128];
  uint32_t pkt_size = cfg->packet_size;
  if (pkt_size == 0)
    pkt_size = nipc_uds_detect_packet_size(fd);

  nipc_hello_t hello = {
      .layout_version = 1,
      .supported_profiles = cfg->supported_profiles ? cfg->supported_profiles
                                                    : NIPC_PROFILE_BASELINE,
      .preferred_profiles = cfg->preferred_profiles,
      .max_request_payload_bytes = apply_default(cfg->max_request_payload_bytes,
                                                 NIPC_MAX_PAYLOAD_DEFAULT),
      .max_request_batch_items =
          apply_default(cfg->max_request_batch_items, UDS_DEFAULT_BATCH_ITEMS),
      .max_response_payload_bytes = apply_default(
          cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
      .max_response_batch_items =
          apply_default(cfg->max_response_batch_items, UDS_DEFAULT_BATCH_ITEMS),
      .auth_token = cfg->auth_token,
      .packet_size = pkt_size,
  };

  uint8_t hello_buf[44];
  nipc_hello_encode(&hello, hello_buf, sizeof(hello_buf));

  nipc_header_t hdr;
  encode_control_header(&hdr, NIPC_CODE_HELLO, NIPC_STATUS_OK,
                        sizeof(hello_buf));
  nipc_header_encode(&hdr, buf, sizeof(buf));
  memcpy(buf + NIPC_HEADER_LEN, hello_buf, sizeof(hello_buf));

  nipc_uds_error_t err =
      nipc_uds_raw_send(fd, buf, NIPC_HEADER_LEN + sizeof(hello_buf));
  if (err != NIPC_UDS_OK)
    return err;

  ssize_t n = nipc_uds_raw_recv(fd, buf, sizeof(buf));
  if (n <= 0)
    return NIPC_UDS_ERR_RECV;

  nipc_header_t ack_hdr;
  nipc_error_t perr = nipc_header_decode(buf, (size_t)n, &ack_hdr);
  if (perr == NIPC_ERR_BAD_VERSION)
    return NIPC_UDS_ERR_INCOMPATIBLE;
  if (perr != NIPC_OK)
    return NIPC_UDS_ERR_PROTOCOL;

  if (ack_hdr.kind != NIPC_KIND_CONTROL || ack_hdr.code != NIPC_CODE_HELLO_ACK)
    return NIPC_UDS_ERR_PROTOCOL;

  err = ack_status_to_error(ack_hdr.transport_status);
  if (err != NIPC_UDS_OK)
    return err;

  nipc_hello_ack_t ack;
  perr = nipc_hello_ack_decode(buf + NIPC_HEADER_LEN,
                               (size_t)n - NIPC_HEADER_LEN, &ack);
  if (perr == NIPC_ERR_BAD_LAYOUT &&
      hello_ack_layout_incompatible(buf + NIPC_HEADER_LEN,
                                    (size_t)n - NIPC_HEADER_LEN))
    return NIPC_UDS_ERR_INCOMPATIBLE;
  if (perr != NIPC_OK)
    return NIPC_UDS_ERR_PROTOCOL;

  fill_client_session(session, fd, &ack);
  if (session->packet_size <= NIPC_HEADER_LEN)
    return NIPC_UDS_ERR_PROTOCOL;

  return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_server_handshake(int fd,
                                           const nipc_uds_server_config_t *cfg,
                                           uint64_t session_id,
                                           nipc_uds_session_t *session) {
  uint8_t buf[128];
  uint32_t server_pkt_size = cfg->packet_size;
  if (server_pkt_size == 0)
    server_pkt_size = nipc_uds_detect_packet_size(fd);

  uint32_t s_resp_pay =
      apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
  uint32_t s_req_pay =
      apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
  uint32_t s_profiles =
      cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE;
  uint32_t s_preferred = cfg->preferred_profiles;

  ssize_t n = nipc_uds_raw_recv(fd, buf, sizeof(buf));
  if (n <= 0)
    return NIPC_UDS_ERR_RECV;

  nipc_header_t hdr;
  nipc_error_t perr = nipc_header_decode(buf, (size_t)n, &hdr);
  if (perr == NIPC_ERR_BAD_VERSION &&
      header_version_incompatible(buf, (size_t)n, NIPC_CODE_HELLO)) {
    send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_UDS_ERR_INCOMPATIBLE;
  }
  if (perr != NIPC_OK)
    return NIPC_UDS_ERR_PROTOCOL;

  if (hdr.kind != NIPC_KIND_CONTROL || hdr.code != NIPC_CODE_HELLO)
    return NIPC_UDS_ERR_PROTOCOL;

  nipc_hello_t hello;
  perr = nipc_hello_decode(buf + NIPC_HEADER_LEN, (size_t)n - NIPC_HEADER_LEN,
                           &hello);
  if (perr == NIPC_ERR_BAD_LAYOUT &&
      hello_layout_incompatible(buf + NIPC_HEADER_LEN,
                                (size_t)n - NIPC_HEADER_LEN)) {
    send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_UDS_ERR_INCOMPATIBLE;
  }
  if (perr != NIPC_OK)
    return NIPC_UDS_ERR_PROTOCOL;

  uint32_t intersection = hello.supported_profiles & s_profiles;
  if (intersection == 0) {
    send_rejection_ack(fd, NIPC_STATUS_UNSUPPORTED);
    return NIPC_UDS_ERR_NO_PROFILE;
  }

  if (hello.auth_token != cfg->auth_token) {
    send_rejection_ack(fd, NIPC_STATUS_AUTH_FAILED);
    return NIPC_UDS_ERR_AUTH_FAILED;
  }

  uint32_t preferred_intersection =
      intersection & hello.preferred_profiles & s_preferred;
  uint32_t selected = preferred_intersection != 0
                          ? highest_bit(preferred_intersection)
                          : highest_bit(intersection);

  if (hello.max_request_payload_bytes > s_req_pay) {
    send_rejection_ack(fd, NIPC_STATUS_LIMIT_EXCEEDED);
    return NIPC_UDS_ERR_LIMIT_EXCEEDED;
  }

  uint32_t agreed_req_pay = hello.max_request_payload_bytes;
  uint32_t agreed_req_bat = hello.max_request_batch_items;
  uint32_t agreed_resp_pay = s_resp_pay;
  uint32_t agreed_resp_bat = agreed_req_bat;
  uint32_t agreed_pkt = min_u32(hello.packet_size, server_pkt_size);

  if (agreed_pkt <= NIPC_HEADER_LEN) {
    send_rejection_ack(fd, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_UDS_ERR_INCOMPATIBLE;
  }

  nipc_hello_ack_t ack = {
      .layout_version = 1,
      .server_supported_profiles = s_profiles,
      .intersection_profiles = intersection,
      .selected_profile = selected,
      .agreed_max_request_payload_bytes = agreed_req_pay,
      .agreed_max_request_batch_items = agreed_req_bat,
      .agreed_max_response_payload_bytes = agreed_resp_pay,
      .agreed_max_response_batch_items = agreed_resp_bat,
      .agreed_packet_size = agreed_pkt,
      .session_id = session_id,
  };

  uint8_t ack_buf[48];
  nipc_hello_ack_encode(&ack, ack_buf, sizeof(ack_buf));

  nipc_header_t ack_hdr;
  encode_control_header(&ack_hdr, NIPC_CODE_HELLO_ACK, NIPC_STATUS_OK,
                        sizeof(ack_buf));

  uint8_t pkt[80];
  nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
  memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));

  nipc_uds_error_t send_ack_err =
      nipc_uds_raw_send(fd, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
  if (send_ack_err != NIPC_UDS_OK)
    return send_ack_err;

  fill_server_session(session, fd, selected, &ack);
  return NIPC_UDS_OK;
}
