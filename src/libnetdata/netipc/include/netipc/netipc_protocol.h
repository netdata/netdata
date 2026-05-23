/*
 * netipc_protocol.h - Wire envelope and codec for the netipc protocol.
 *
 * Pure byte-layout encode/decode. No I/O, no transport, no allocation on
 * decode. Localhost-only IPC — all multi-byte fields use host byte order.
 * Struct layouts match wire format exactly; encode/decode is a single memcpy
 * plus validation.
 *
 * Decoded "View" types borrow the underlying buffer and are valid only while
 * that buffer lives. Copy immediately if the data is needed later.
 */

#ifndef NETIPC_PROTOCOL_H
#define NETIPC_PROTOCOL_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Constants                                                         */
/* ------------------------------------------------------------------ */

#define NIPC_MAGIC_MSG   0x4e495043u  /* "NIPC" */
#define NIPC_MAGIC_CHUNK 0x4e43484bu  /* "NCHK" */
#define NIPC_VERSION     1u
#define NIPC_HEADER_LEN  32u

/* Message kinds */
#define NIPC_KIND_REQUEST  1u
#define NIPC_KIND_RESPONSE 2u
#define NIPC_KIND_CONTROL  3u

/* Flags */
#define NIPC_FLAG_BATCH 0x0001u

/* Transport status */
#define NIPC_STATUS_OK              0u
#define NIPC_STATUS_BAD_ENVELOPE    1u
#define NIPC_STATUS_AUTH_FAILED     2u
#define NIPC_STATUS_INCOMPATIBLE   3u
#define NIPC_STATUS_UNSUPPORTED    4u
#define NIPC_STATUS_LIMIT_EXCEEDED 5u
#define NIPC_STATUS_INTERNAL_ERROR 6u

/* Control opcodes */
#define NIPC_CODE_HELLO     1u
#define NIPC_CODE_HELLO_ACK 2u

/* Method codes */
#define NIPC_METHOD_INCREMENT        1u
#define NIPC_METHOD_CGROUPS_SNAPSHOT 2u
#define NIPC_METHOD_STRING_REVERSE   3u

/* Profile bits */
#define NIPC_PROFILE_BASELINE    0x01u
#define NIPC_PROFILE_SHM_HYBRID  0x02u
#define NIPC_PROFILE_SHM_FUTEX   0x04u
#define NIPC_PROFILE_SHM_WAITADDR 0x08u

/* Defaults */
#define NIPC_MAX_PAYLOAD_DEFAULT 1024u

/* Hard cap on negotiated request payload sizes (1 MiB) — prevents a
 * compromised peer from forcing excessive memory allocation. */
#define NIPC_MAX_PAYLOAD_CAP (1024u * 1024u)

/* Alignment for batch items and cgroups items */
#define NIPC_ALIGNMENT 8u

/* ------------------------------------------------------------------ */
/*  Error codes                                                       */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_OK = 0,
    NIPC_ERR_TRUNCATED,       /* buffer too short for the expected structure */
    NIPC_ERR_BAD_MAGIC,       /* magic value mismatch */
    NIPC_ERR_BAD_VERSION,     /* unsupported version */
    NIPC_ERR_BAD_HEADER_LEN,  /* header_len != 32 */
    NIPC_ERR_BAD_KIND,        /* unknown message kind */
    NIPC_ERR_BAD_LAYOUT,      /* unknown layout_version in a payload */
    NIPC_ERR_OUT_OF_BOUNDS,   /* offset+length exceeds available data */
    NIPC_ERR_MISSING_NUL,     /* string not NUL-terminated */
    NIPC_ERR_BAD_ALIGNMENT,   /* item not 8-byte aligned */
    NIPC_ERR_BAD_ITEM_COUNT,  /* directory inconsistent with payload size */
    NIPC_ERR_OVERFLOW,        /* builder ran out of space */
    NIPC_ERR_HANDLER_FAILED,  /* typed handler rejected an otherwise valid request */
    NIPC_ERR_NOT_READY,       /* client not connected / service unavailable */
} nipc_error_t;

/* ------------------------------------------------------------------ */
/*  Outer message header (32 bytes)                                   */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t magic;
    uint16_t version;
    uint16_t header_len;
    uint16_t kind;
    uint16_t flags;
    uint16_t code;
    uint16_t transport_status;
    uint32_t payload_len;
    uint32_t item_count;
    uint64_t message_id;
} nipc_header_t;

/* Encode header into buf (must be >= 32 bytes). Returns 32. */
size_t nipc_header_encode(const nipc_header_t *hdr, void *buf, size_t buf_len);

/* Decode header from buf. Returns NIPC_OK or an error. */
nipc_error_t nipc_header_decode(const void *buf, size_t buf_len,
                                nipc_header_t *out);

/* ------------------------------------------------------------------ */
/*  Chunk continuation header (32 bytes)                              */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t magic;
    uint16_t version;
    uint16_t flags;
    uint64_t message_id;
    uint32_t total_message_len;
    uint32_t chunk_index;
    uint32_t chunk_count;
    uint32_t chunk_payload_len;
} nipc_chunk_header_t;

size_t nipc_chunk_header_encode(const nipc_chunk_header_t *chk,
                                void *buf, size_t buf_len);

nipc_error_t nipc_chunk_header_decode(const void *buf, size_t buf_len,
                                      nipc_chunk_header_t *out);

/* ------------------------------------------------------------------ */
/*  Batch item directory                                              */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t offset;
    uint32_t length;
} nipc_batch_entry_t;

/*
 * Encode item_count directory entries into buf.
 * Returns total bytes written (item_count * 8).
 */
size_t nipc_batch_dir_encode(const nipc_batch_entry_t *entries,
                             uint32_t item_count,
                             void *buf, size_t buf_len);

/*
 * Decode and validate item_count directory entries from buf.
 * packed_area_len is the size of the packed item area that follows
 * the directory. Each entry's offset+length must fall within it.
 * Returns NIPC_OK or an error.
 */
nipc_error_t nipc_batch_dir_decode(const void *buf, size_t buf_len,
                                   uint32_t item_count,
                                   uint32_t packed_area_len,
                                   nipc_batch_entry_t *out);

/*
 * Validate a batch directory without allocating an output array.
 * Checks alignment and bounds for each entry. For use in L1 receive
 * paths where allocation is undesirable.
 * buf/buf_len: the directory bytes (item_count * 8).
 * packed_area_len: size of the packed item area after the directory.
 */
nipc_error_t nipc_batch_dir_validate(const void *buf, size_t buf_len,
                                      uint32_t item_count,
                                      uint32_t packed_area_len);

/*
 * Extract a single batch item by index from a complete batch payload.
 * payload points to the first byte after the outer header.
 * On success, *item_ptr and *item_len are set.
 */
nipc_error_t nipc_batch_item_get(const void *payload, size_t payload_len,
                                 uint32_t item_count, uint32_t index,
                                 const void **item_ptr, uint32_t *item_len);

/* ------------------------------------------------------------------ */
/*  Batch builder                                                     */
/* ------------------------------------------------------------------ */

typedef struct {
    uint8_t  *buf;         /* caller-owned output buffer */
    size_t    buf_len;     /* total buffer capacity */
    uint32_t  item_count;  /* items added so far */
    uint32_t  max_items;   /* max items (from directory capacity) */
    size_t    dir_end;     /* byte offset where directory ends */
    size_t    data_offset; /* current write position in packed area */
} nipc_batch_builder_t;

/*
 * Initialize a batch builder.
 * buf must be large enough for: max_items*8 (directory) + packed data.
 * The directory is written at the front; packed data grows after it.
 */
void nipc_batch_builder_init(nipc_batch_builder_t *b,
                             void *buf, size_t buf_len,
                             uint32_t max_items);

/* Add an item payload. Returns NIPC_OK or NIPC_ERR_OVERFLOW. */
nipc_error_t nipc_batch_builder_add(nipc_batch_builder_t *b,
                                    const void *item, size_t item_len);

/*
 * Finalize: writes the directory entries and returns the total payload
 * size (directory + packed items). Sets *item_count_out.
 */
size_t nipc_batch_builder_finish(nipc_batch_builder_t *b,
                                 uint32_t *item_count_out);

/* ------------------------------------------------------------------ */
/*  Hello payload (44 bytes)                                          */
/* ------------------------------------------------------------------ */

typedef struct {
    uint16_t layout_version;
    uint16_t flags;
    uint32_t supported_profiles;
    uint32_t preferred_profiles;
    uint32_t max_request_payload_bytes;
    uint32_t max_request_batch_items;
    uint32_t max_response_payload_bytes;
    uint32_t max_response_batch_items;
    uint32_t _reserved;  /* wire offset 28, must be 0 */
    uint64_t auth_token;
    uint32_t packet_size;
} nipc_hello_t;

/* Wire size (44) differs from sizeof (48) due to trailing alignment. */
#define NIPC_HELLO_WIRE_SIZE 44u

size_t nipc_hello_encode(const nipc_hello_t *h, void *buf, size_t buf_len);

nipc_error_t nipc_hello_decode(const void *buf, size_t buf_len,
                               nipc_hello_t *out);

/* ------------------------------------------------------------------ */
/*  Hello-ack payload (48 bytes)                                      */
/* ------------------------------------------------------------------ */

typedef struct {
    uint16_t layout_version;
    uint16_t flags;
    uint32_t server_supported_profiles;
    uint32_t intersection_profiles;
    uint32_t selected_profile;
    uint32_t agreed_max_request_payload_bytes;
    uint32_t agreed_max_request_batch_items;
    uint32_t agreed_max_response_payload_bytes;
    uint32_t agreed_max_response_batch_items;
    uint32_t agreed_packet_size;
    uint32_t _reserved;   /* wire offset 36, must be 0 */
    uint64_t session_id;  /* server-assigned, for per-session SHM path */
} nipc_hello_ack_t;

size_t nipc_hello_ack_encode(const nipc_hello_ack_t *h,
                             void *buf, size_t buf_len);

nipc_error_t nipc_hello_ack_decode(const void *buf, size_t buf_len,
                                   nipc_hello_ack_t *out);

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot request (4 bytes)                                */
/* ------------------------------------------------------------------ */

typedef struct {
    uint16_t layout_version;
    uint16_t flags;
} nipc_cgroups_req_t;

size_t nipc_cgroups_req_encode(const nipc_cgroups_req_t *r,
                               void *buf, size_t buf_len);

nipc_error_t nipc_cgroups_req_decode(const void *buf, size_t buf_len,
                                     nipc_cgroups_req_t *out);

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot response                                         */
/* ------------------------------------------------------------------ */

/* Snapshot-level header (24 bytes) */
typedef struct {
    uint16_t layout_version;
    uint16_t flags;
    uint32_t item_count;
    uint32_t systemd_enabled;
    uint32_t reserved;
    uint64_t generation;
} nipc_cgroups_resp_header_t;

/* Borrowed string view into the payload buffer. */
typedef struct {
    const char *ptr;   /* points into payload, NUL-terminated */
    uint32_t    len;   /* length excluding the NUL */
} nipc_str_view_t;

/*
 * Per-item view -- ephemeral, borrows the payload buffer.
 * Valid only while the payload buffer is alive.
 */
typedef struct {
    uint16_t        layout_version;
    uint16_t        flags;
    uint32_t        hash;
    uint32_t        options;
    uint32_t        enabled;
    nipc_str_view_t name;
    nipc_str_view_t path;
} nipc_cgroups_item_view_t;

/* Full snapshot view -- ephemeral. */
typedef struct {
    uint16_t    layout_version;
    uint16_t    flags;
    uint32_t    item_count;
    uint32_t    systemd_enabled;
    uint64_t    generation;

    /* Internal: payload pointer and size for item access */
    const uint8_t *_payload;
    size_t         _payload_len;
} nipc_cgroups_resp_view_t;

/*
 * Decode the snapshot response header and validate the item directory.
 * On success, use nipc_cgroups_resp_item() to access individual items.
 */
nipc_error_t nipc_cgroups_resp_decode(const void *buf, size_t buf_len,
                                      nipc_cgroups_resp_view_t *out);

/*
 * Access item at index from a decoded snapshot view.
 * index must be < view->item_count.
 */
nipc_error_t nipc_cgroups_resp_item(const nipc_cgroups_resp_view_t *view,
                                    uint32_t index,
                                    nipc_cgroups_item_view_t *out);

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot response builder                                 */
/* ------------------------------------------------------------------ */

#define NIPC_CGROUPS_ITEM_HDR_SIZE 32u
#define NIPC_CGROUPS_RESP_HDR_SIZE 24u
#define NIPC_CGROUPS_DIR_ENTRY_SIZE 8u

typedef struct {
    uint8_t  *buf;
    size_t    buf_len;
    uint32_t  systemd_enabled;
    uint64_t  generation;
    uint32_t  item_count;
    uint32_t  max_items;  /* directory slots reserved at init */
    nipc_error_t error;   /* sticky builder failure for dispatch */

    /* Current write position for packed item data (absolute). */
    size_t    data_offset;
} nipc_cgroups_builder_t;

/*
 * Initialize the builder. buf must be caller-owned and large enough
 * for the expected snapshot. max_items is a hint for directory space
 * reservation.
 */
void nipc_cgroups_builder_init(nipc_cgroups_builder_t *b,
                               void *buf, size_t buf_len,
                               uint32_t max_items,
                               uint32_t systemd_enabled,
                               uint64_t generation);

/* Update the snapshot header fields that finish() writes. */
void nipc_cgroups_builder_set_header(nipc_cgroups_builder_t *b,
                                     uint32_t systemd_enabled,
                                     uint64_t generation);

/* Return a safe upper bound for the number of snapshot items that can
 * fit in a response buffer of size buf_len. This is for directory
 * reservation only, not a promise for arbitrary string sizes. */
uint32_t nipc_cgroups_builder_estimate_max_items(size_t buf_len);

/*
 * Add one cgroup item. The builder handles offset bookkeeping,
 * NUL termination, and alignment.
 */
nipc_error_t nipc_cgroups_builder_add(nipc_cgroups_builder_t *b,
                                      uint32_t hash,
                                      uint32_t options,
                                      uint32_t enabled,
                                      const char *name, uint32_t name_len,
                                      const char *path, uint32_t path_len);

/*
 * Finalize the builder. Writes the snapshot header and returns the
 * total payload size. The buffer now contains a complete, decodable
 * cgroups snapshot response payload.
 */
size_t nipc_cgroups_builder_finish(nipc_cgroups_builder_t *b);

/* ------------------------------------------------------------------ */
/*  INCREMENT codec (8 bytes)                                         */
/* ------------------------------------------------------------------ */

/*
 * INCREMENT payload: { uint64_t value }.
 * Same layout for both request and response.
 */
#define NIPC_INCREMENT_PAYLOAD_SIZE 8u

/* Encode value into buf. Returns 8 on success, 0 if buf too small. */
size_t nipc_increment_encode(uint64_t value, void *buf, size_t buf_len);

/* Decode value from buf. Returns NIPC_OK or NIPC_ERR_TRUNCATED. */
nipc_error_t nipc_increment_decode(const void *buf, size_t buf_len,
                                    uint64_t *value_out);

/* ------------------------------------------------------------------ */
/*  STRING_REVERSE codec (variable length)                            */
/* ------------------------------------------------------------------ */

/*
 * STRING_REVERSE payload wire layout:
 *   | 0 | 4 | u32  | str_offset (from payload start, always 8) |
 *   | 4 | 4 | u32  | str_length (excluding NUL)                 |
 *   | 8 | N+1 | bytes | string data + NUL                       |
 *
 * Same layout for both request and response.
 * Total size = 8 + str_length + 1.
 */
#define NIPC_STRING_REVERSE_HDR_SIZE 8u

/* Ephemeral view into a decoded STRING_REVERSE payload.
 * Borrows the payload buffer — valid only during the current call. */
typedef struct {
    const char *str;   /* pointer into payload, NUL-terminated */
    uint32_t str_len;  /* length excluding NUL */
} nipc_string_reverse_view_t;

/* Encode str (str_len bytes) into buf. Returns total bytes written,
 * or 0 if buf too small. Appends trailing NUL. */
size_t nipc_string_reverse_encode(const char *str, uint32_t str_len,
                                   void *buf, size_t buf_len);

/* Decode payload into an ephemeral view. Validates bounds and NUL.
 * Returns NIPC_OK or error. */
nipc_error_t nipc_string_reverse_decode(const void *buf, size_t buf_len,
                                         nipc_string_reverse_view_t *view_out);

/* ------------------------------------------------------------------ */
/*  Server-side typed dispatch helpers                                 */
/* ------------------------------------------------------------------ */

/*
 * Per-method dispatch: decode request → call typed handler → encode
 * response. Handlers never touch wire format — pure business logic.
 *
 * Each helper takes (raw_request, raw_response_buf, handler_fn, user).
 * Returns true on success (response written), false on failure.
 */

/* INCREMENT: handler receives decoded u64, returns u64. */
typedef bool (*nipc_increment_handler_fn)(
    void *user, uint64_t request, uint64_t *response);

bool nipc_dispatch_increment(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_increment_handler_fn handler, void *user);

/* STRING_REVERSE: handler receives decoded string, writes response string. */
typedef bool (*nipc_string_reverse_handler_fn)(
    void *user,
    const char *request_str, uint32_t request_str_len,
    char *response_str, uint32_t response_capacity,
    uint32_t *response_str_len);

bool nipc_dispatch_string_reverse(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_string_reverse_handler_fn handler, void *user);

/* CGROUPS_SNAPSHOT: handler receives decoded request, fills builder. */
typedef bool (*nipc_cgroups_handler_fn)(
    void *user,
    const nipc_cgroups_req_t *request,
    nipc_cgroups_builder_t *builder);

nipc_error_t nipc_dispatch_cgroups_snapshot(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    uint32_t max_items,
    nipc_cgroups_handler_fn handler, void *user);

/* ------------------------------------------------------------------ */
/*  Utility: 8-byte alignment                                         */
/* ------------------------------------------------------------------ */

static inline size_t nipc_align8(size_t v) {
    return (v + 7u) & ~(size_t)7u;
}

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_PROTOCOL_H */
