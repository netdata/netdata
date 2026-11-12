/*
 * netipc_named_pipe.c - L1 Windows Named Pipe transport.
 *
 * Implements connection lifecycle, handshake with profile/limit negotiation,
 * and send/receive with transparent chunking over Win32 Named Pipes in
 * message mode. Wire-compatible with all language implementations.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_protocol.h"
#include "netipc_win_security.h"
#include "netipc/netipc_win_shm.h"

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Profiles Windows can provide; unassigned bits are never offered or
 * accepted, so a peer can never select a profile without an implementation
 * (0x04 is WinSHM busy-wait here, not the POSIX futex profile). */
#define NIPC_NP_PLATFORM_PROFILES                                              \
  (NIPC_PROFILE_BASELINE | NIPC_WIN_SHM_PROFILE_HYBRID |                       \
   NIPC_WIN_SHM_PROFILE_BUSYWAIT)

/* ------------------------------------------------------------------ */
/*  Win32 error constants for disconnect detection                     */
/* ------------------------------------------------------------------ */

#ifndef ERROR_BROKEN_PIPE
#define ERROR_BROKEN_PIPE 109
#endif
#ifndef ERROR_NO_DATA
#define ERROR_NO_DATA 232
#endif
#ifndef ERROR_PIPE_NOT_CONNECTED
#define ERROR_PIPE_NOT_CONNECTED 233
#endif

/* ------------------------------------------------------------------ */
/*  Internal helpers                                                   */
/* ------------------------------------------------------------------ */

static inline uint32_t min_u32(uint32_t a, uint32_t b) { return a < b ? a : b; }

static inline uint32_t max_u32(uint32_t a, uint32_t b) { return a > b ? a : b; }

static inline uint32_t apply_default(uint32_t val, uint32_t def) {
  return val == 0 ? def : val;
}

static bool header_payload_len(size_t payload_len, size_t *msg_len_out) {
  if (payload_len > SIZE_MAX - NIPC_HEADER_LEN)
    return false;

  size_t msg_len = NIPC_HEADER_LEN + payload_len;
  if (msg_len > UINT32_MAX)
    return false;

  *msg_len_out = msg_len;
  return true;
}

static inline uint32_t pipe_buffer_size(uint32_t packet_size) {
  /* The protocol packet size controls logical framing and chunk size. The
   * underlying pipe quota must stay large enough for full-duplex pipelining
   * even when tests force a tiny protocol packet size. */
  return max_u32(apply_default(packet_size, NIPC_NP_DEFAULT_PIPE_BUF_SIZE),
                 NIPC_NP_DEFAULT_PIPE_BUF_SIZE);
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

static nipc_np_error_t raw_send(HANDLE pipe, const void *data, size_t len);

static void send_rejection_ack(HANDLE pipe, uint16_t status) {
  nipc_hello_ack_t ack = {.layout_version = 1};
  uint8_t ack_buf[48];
  uint8_t pkt[80];
  nipc_header_t ack_hdr = {
      .magic = NIPC_MAGIC_MSG,
      .version = NIPC_VERSION,
      .header_len = NIPC_HEADER_LEN,
      .kind = NIPC_KIND_CONTROL,
      .code = NIPC_CODE_HELLO_ACK,
      .transport_status = status,
      .payload_len = sizeof(ack_buf),
      .item_count = 1,
  };

  nipc_hello_ack_encode(&ack, ack_buf, sizeof(ack_buf));
  nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
  memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));
  raw_send(pipe, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
}

/* Highest set bit in a bitmask (0 if empty). */
static uint32_t highest_bit(uint32_t mask) {
  if (mask == 0)
    return 0;

  uint32_t bit = 1u << 31;
  while (!(mask & bit))
    bit >>= 1;
  return bit;
}

/* ------------------------------------------------------------------ */
/*  FNV-1a 64-bit hash                                                 */
/* ------------------------------------------------------------------ */

uint64_t nipc_fnv1a_64(const void *data, size_t len) {
  uint64_t hash = NIPC_FNV1A_OFFSET_BASIS;
  const uint8_t *p = (const uint8_t *)data;

  for (size_t i = 0; i < len; i++) {
    hash ^= (uint64_t)p[i];
    hash *= NIPC_FNV1A_PRIME;
  }

  return hash;
}

/* ------------------------------------------------------------------ */
/*  Service name validation                                            */
/* ------------------------------------------------------------------ */

/* Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..". */
static int validate_service_name(const char *name) {
  if (!name || name[0] == '\0')
    return -1;

  /* Reject "." and ".." */
  if (name[0] == '.' &&
      (name[1] == '\0' || (name[1] == '.' && name[2] == '\0')))
    return -1;

  for (const char *p = name; *p; p++) {
    char c = *p;
    if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
        (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-')
      continue;
    return -1;
  }
  return 0;
}

/* ------------------------------------------------------------------ */
/*  Pipe name derivation                                               */
/* ------------------------------------------------------------------ */

int nipc_np_build_pipe_name(wchar_t *dst, size_t dst_chars, const char *run_dir,
                            const char *service_name) {
  if (!dst || dst_chars == 0 || !run_dir || !service_name)
    return -1;

  if (validate_service_name(service_name) < 0)
    return -1;

  /* Hash the run_dir */
  uint64_t hash = nipc_fnv1a_64(run_dir, strlen(run_dir));

  /* Build the pipe name as narrow first, then widen */
  char narrow[NIPC_NP_MAX_PIPE_NAME];
  int n = snprintf(narrow, sizeof(narrow), "\\\\.\\pipe\\netipc-%016llx-%s",
                   (unsigned long long)hash, service_name);
  if (n < 0 || (size_t)n >= sizeof(narrow))
    return -1;

  /* Convert to wide string */
  if ((size_t)(n + 1) > dst_chars)
    return -1;

  for (int i = 0; i <= n; i++)
    dst[i] = (wchar_t)(unsigned char)narrow[i];

  return 0;
}

/* ------------------------------------------------------------------ */
/*  Create a new pipe instance                                         */
/* ------------------------------------------------------------------ */

static HANDLE create_pipe_instance(const wchar_t *pipe_name, uint32_t buf_size,
                                   BOOL first_instance) {
  DWORD open_mode = PIPE_ACCESS_DUPLEX;
  if (first_instance)
    open_mode |= FILE_FLAG_FIRST_PIPE_INSTANCE;

  SECURITY_ATTRIBUTES sa;
  if (!nipc_win_security_attrs_init(&sa))
    return INVALID_HANDLE_VALUE;

  HANDLE h = CreateNamedPipeW(pipe_name, open_mode,
                              PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE |
                                  PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
                              NIPC_NP_MAX_INSTANCES, buf_size, buf_size,
                              0, /* default timeout */
                              &sa);
  DWORD err = GetLastError();
  nipc_win_security_attrs_free(&sa);
  SetLastError(err);

  return h;
}

/* ------------------------------------------------------------------ */
/*  Low-level send / recv                                              */
/* ------------------------------------------------------------------ */

/* Check if a Win32 error indicates peer disconnect (graceful). */
static int is_disconnect_error(DWORD err) {
  return err == ERROR_BROKEN_PIPE || err == ERROR_NO_DATA ||
         err == ERROR_PIPE_NOT_CONNECTED;
}

/* Write exactly len bytes as one pipe message. */
static nipc_np_error_t raw_send(HANDLE pipe, const void *data, size_t len) {
  DWORD written = 0;
  BOOL ok = WriteFile(pipe, data, (DWORD)len, &written, NULL);
  if (!ok) {
    if (is_disconnect_error(GetLastError()))
      return NIPC_NP_ERR_DISCONNECTED;
    return NIPC_NP_ERR_SEND;
  }
  if (written != (DWORD)len)
    return NIPC_NP_ERR_SEND;
  return NIPC_NP_OK;
}

typedef struct receive_wait receive_wait_t;
static nipc_np_error_t pipe_send(nipc_np_session_t *s, const void *data,
                                 size_t len, const receive_wait_t *wait);

/* Send header + payload as one pipe message (concatenated). */
static nipc_np_error_t raw_send_msg(nipc_np_session_t *session, const void *hdr,
                                    size_t hdr_len, const void *payload,
                                    size_t payload_len,
                                    const receive_wait_t *wait) {
  /* Named Pipes don't support scatter-gather like sendmsg.
   * Concatenate into one buffer for a single WriteFile call. */
  size_t total = hdr_len + payload_len;
  uint8_t stack_buf[256];
  uint8_t *buf;
  int allocated = 0;

  if (total <= sizeof(stack_buf)) {
    buf = stack_buf;
  } else {
    buf = (uint8_t *)malloc(total);
    if (!buf)
      return NIPC_NP_ERR_ALLOC;
    allocated = 1;
  }

  memcpy(buf, hdr, hdr_len);
  if (payload && payload_len > 0)
    memcpy(buf + hdr_len, payload, payload_len);

  nipc_np_error_t err = pipe_send(session, buf, total, wait);

  if (allocated)
    free(buf);

  return err;
}

/* Read one pipe message. Returns bytes read, 0 on disconnect, -1 on error. */
static int raw_recv(HANDLE pipe, void *buf, size_t buf_len, DWORD *bytes_read) {
  DWORD read = 0;
  BOOL ok = ReadFile(pipe, buf, (DWORD)buf_len, &read, NULL);
  if (!ok) {
    DWORD err = GetLastError();
    if (is_disconnect_error(err)) {
      *bytes_read = 0;
      return 0; /* disconnect */
    }
    return -1; /* error */
  }
  if (read == 0) {
    *bytes_read = 0;
    return 0; /* disconnect */
  }
  *bytes_read = read;
  return 1; /* success */
}

/* Client pipe handles are overlapped (FILE_FLAG_OVERLAPPED), so a pipelining
 * send can read responses while its write waits for buffer space. Every read
 * and write on such a handle passes an OVERLAPPED and is waited to completion
 * before the call returns, so the OVERLAPPED lives on that call's stack.
 * Server handles stay synchronous. */

static nipc_np_error_t send_error(DWORD err) {
  return is_disconnect_error(err) ? NIPC_NP_ERR_DISCONNECTED : NIPC_NP_ERR_SEND;
}

/* Read one message on an overlapped handle and wait for it; same returns as
 * raw_recv(). A message larger than buf (ERROR_MORE_DATA) fails, as on a
 * synchronous handle. */
static int overlapped_recv(HANDLE pipe, HANDLE event, void *buf, size_t buf_len,
                           DWORD *bytes_read) {
  OVERLAPPED ov = {0};
  ov.hEvent = event;
  DWORD read = 0;
  *bytes_read = 0;
  /* Overlapped I/O takes no byte-count pointer (Win32 docs); the count
   * comes from GetOverlappedResult. */
  if (!ReadFile(pipe, buf, (DWORD)buf_len, NULL, &ov)) {
    DWORD err = GetLastError();
    if (err != ERROR_IO_PENDING)
      return is_disconnect_error(err) ? 0 : -1;
  }
  if (!GetOverlappedResult(pipe, &ov, &read, TRUE))
    return is_disconnect_error(GetLastError()) ? 0 : -1;
  if (read == 0)
    return 0;
  *bytes_read = read;
  return 1;
}

/* Read one message with the session's handle kind. */
static int pipe_recv(const nipc_np_session_t *s, void *buf, size_t buf_len,
                     DWORD *bytes_read) {
  if (s->read_event)
    return overlapped_recv(s->pipe, s->read_event, buf, buf_len, bytes_read);
  return raw_recv(s->pipe, buf, buf_len, bytes_read);
}

/* Post one message; finish_overlapped_send() collects its completion, also
 * when WriteFile completed at once. */
static nipc_np_error_t start_overlapped_send(HANDLE pipe, OVERLAPPED *ov,
                                             const void *data, size_t len) {
  /* No byte-count pointer: the write may still be pending when this
   * returns; finish_overlapped_send() gets the count. */
  if (!WriteFile(pipe, data, (DWORD)len, NULL, ov)) {
    DWORD err = GetLastError();
    if (err != ERROR_IO_PENDING)
      return send_error(err);
  }
  return NIPC_NP_OK;
}

static nipc_np_error_t finish_overlapped_send(HANDLE pipe, OVERLAPPED *ov,
                                              size_t len) {
  DWORD written = 0;
  if (!GetOverlappedResult(pipe, ov, &written, TRUE))
    return send_error(GetLastError());
  return written == (DWORD)len ? NIPC_NP_OK : NIPC_NP_ERR_SEND;
}

/* Cancel a posted write and wait until the kernel no longer uses its
 * OVERLAPPED and buffer. */
static void cancel_overlapped_send(HANDLE pipe, OVERLAPPED *ov) {
  DWORD written = 0;
  CancelIoEx(pipe, ov);
  GetOverlappedResult(pipe, ov, &written, TRUE);
}

#define NP_PENDING_RECORD_LEN 4u

static void pending_compact(nipc_np_session_t *s) {
  if (s->pending_head == 0)
    return;
  size_t left = s->pending_len - s->pending_head;
  memmove(s->pending_buf, s->pending_buf + s->pending_head, left);
  s->pending_len = left;
  s->pending_head = 0;
}

/* Pop the oldest queued message into buf; same returns as raw_recv(). A
 * message larger than buf fails, as a pipe read would with
 * ERROR_MORE_DATA. */
static int pending_pop(nipc_np_session_t *s, void *buf, size_t buf_len,
                       DWORD *bytes_read) {
  uint32_t rec;
  memcpy(&rec, s->pending_buf + s->pending_head, sizeof(rec));
  size_t start = s->pending_head + NP_PENDING_RECORD_LEN;
  int rc = -1;
  *bytes_read = 0;
  if ((size_t)rec <= buf_len) {
    memcpy(buf, s->pending_buf + start, rec);
    *bytes_read = rec;
    rc = 1;
  }
  s->pending_head = start + rec;
  if (s->pending_head == s->pending_len)
    s->pending_head = s->pending_len = 0;
  return rc;
}

static int peek_available(HANDLE pipe, DWORD *available) {
  *available = 0;
  return PeekNamedPipe(pipe, NULL, 0, NULL, available, NULL) ? 0 : -1;
}

/* Read every message available now into the pending queue. Sets *drained if
 * it read any. A disconnected pipe sets *eof: the posted write reports it. Queued bytes beyond progress_bound fail with
 * NIPC_NP_ERR_PROTOCOL. */
static nipc_np_error_t pending_drain(nipc_np_session_t *s, bool *drained,
                                     bool *eof) {
  size_t read_len = s->packet_size;
  for (;;) {
    DWORD available = 0;
    if (peek_available(s->pipe, &available) < 0) {
      if (is_disconnect_error(GetLastError())) {
        *eof = true;
        return NIPC_NP_OK;
      }
      /* Stopping the drain on a live pipe would bring the deadlock back. */
      return NIPC_NP_ERR_PROTOCOL;
    }
    if (available == 0)
      return NIPC_NP_OK;

    pending_compact(s);
    size_t need = s->pending_len + NP_PENDING_RECORD_LEN + read_len;
    if (need < s->pending_len)
      return NIPC_NP_ERR_PROTOCOL;
    if (need > s->pending_cap) {
      size_t cap = s->pending_cap ? s->pending_cap : 4096;
      while (cap < need) {
        if (cap > SIZE_MAX / 2)
          return NIPC_NP_ERR_ALLOC;
        cap *= 2;
      }
      uint8_t *p = realloc(s->pending_buf, cap);
      if (!p)
        return NIPC_NP_ERR_ALLOC;
      s->pending_buf = p;
      s->pending_cap = cap;
    }

    size_t start = s->pending_len;
    DWORD n = 0;
    int rc = overlapped_recv(s->pipe, s->read_event,
                             s->pending_buf + start + NP_PENDING_RECORD_LEN,
                             read_len, &n);
    if (rc == 0) {
      *eof = true;
      return NIPC_NP_OK;
    }
    if (rc < 0)
      return NIPC_NP_ERR_PROTOCOL; /* larger than a packet, or a read failure */
    uint32_t rec = (uint32_t)n;
    memcpy(s->pending_buf + start, &rec, sizeof(rec));
    s->pending_len = start + NP_PENDING_RECORD_LEN + n;
    if (s->pending_len > s->progress_bound)
      return NIPC_NP_ERR_PROTOCOL;
    *drained = true;
  }
}

static void inflight_fail_all(nipc_np_session_t *s);

/* A cancelled write leaves the message stream in an unknown state, and a
 * drain failure drops inbound messages: close the pipe, so later calls fail
 * with NIPC_NP_ERR_BAD_PARAM, and fail every in-flight request.
 * nipc_np_close_session() still releases the rest. */
static void break_session(nipc_np_session_t *s) {
  CloseHandle(s->pipe);
  s->pipe = INVALID_HANDLE_VALUE;
  s->pending_len = s->pending_head = 0;
  inflight_fail_all(s);
}

/* Wire bytes `inflight` responses can occupy: the pending queue bound. */
static size_t pending_bound(uint32_t max_response_payload, uint32_t packet_size,
                            uint32_t inflight) {
  size_t payload = max_response_payload;
  size_t budget =
      packet_size > NIPC_HEADER_LEN ? packet_size - NIPC_HEADER_LEN : 1;
  size_t packets = payload == 0 ? 1 : 1 + (payload - 1) / budget;
  size_t per_packet = NIPC_HEADER_LEN + NP_PENDING_RECORD_LEN;
  if (packets > (SIZE_MAX - payload) / per_packet)
    return SIZE_MAX;
  size_t per_response = payload + packets * per_packet;
  if (per_response > SIZE_MAX / inflight)
    return SIZE_MAX;
  return per_response * inflight;
}

struct receive_wait {
  bool infinite;
  ULONGLONG deadline;
  HANDLE abort_event;
};


static receive_wait_t receive_wait_init(uint32_t timeout_ms,
                                        HANDLE abort_event) {
  receive_wait_t wait = {
      .infinite = (timeout_ms == 0),
      .deadline = 0,
      .abort_event = abort_event,
  };
  if (!wait.infinite)
    wait.deadline = GetTickCount64() + timeout_ms;
  return wait;
}

static bool receive_wait_expired(const receive_wait_t *wait) {
  return !wait->infinite && GetTickCount64() >= wait->deadline;
}

static bool receive_wait_aborted(const receive_wait_t *wait) {
  return wait->abort_event &&
         WaitForSingleObject(wait->abort_event, 0) == WAIT_OBJECT_0;
}

/* How long a stalled pipelined send keeps yielding before it sleeps. */
#define NP_PROGRESS_YIELD_US 1000u

static ULONGLONG progress_now_us(void) {
  LARGE_INTEGER count, freq;
  QueryPerformanceCounter(&count);
  QueryPerformanceFrequency(&freq);
  ULONGLONG c = (ULONGLONG)count.QuadPart, f = (ULONGLONG)freq.QuadPart;
  return c / f * 1000000u + c % f * 1000000u / f;
}

/* A posted write that ends on the deadline or abort is cancelled and the
 * session broken, since part of the message may be on the wire. */
static nipc_np_error_t send_wait_ended(nipc_np_session_t *s, OVERLAPPED *ov,
                                       const receive_wait_t *wait) {
  nipc_np_error_t err = NIPC_NP_OK;
  if (receive_wait_aborted(wait))
    err = NIPC_NP_ERR_ABORTED;
  else if (receive_wait_expired(wait))
    err = NIPC_NP_ERR_TIMEOUT;
  if (err != NIPC_NP_OK) {
    cancel_overlapped_send(s->pipe, ov);
    break_session(s);
  }
  return err;
}

/* Wait for a posted write until it completes, the deadline passes or the
 * abort event is signaled. */
static nipc_np_error_t finish_overlapped_send_wait(nipc_np_session_t *s,
                                                   OVERLAPPED *ov, size_t len,
                                                   const receive_wait_t *wait) {
  for (;;) {
    DWORD written = 0;
    if (GetOverlappedResult(s->pipe, ov, &written, FALSE))
      return written == (DWORD)len ? NIPC_NP_OK : NIPC_NP_ERR_SEND;
    DWORD gle = GetLastError();
    if (gle != ERROR_IO_INCOMPLETE)
      return send_error(gle);
    nipc_np_error_t err = send_wait_ended(s, ov, wait);
    if (err != NIPC_NP_OK)
      return err;

    HANDLE handles[2] = {s->write_event, wait->abort_event};
    DWORD count = wait->abort_event ? 2 : 1;
    DWORD ms = INFINITE;
    if (!wait->infinite) {
      ULONGLONG now = GetTickCount64();
      ULONGLONG left = wait->deadline > now ? wait->deadline - now : 0;
      ms = left >= INFINITE ? INFINITE - 1 : (DWORD)left;
    }
    if (WaitForMultipleObjects(count, handles, FALSE, ms) == WAIT_FAILED) {
      cancel_overlapped_send(s->pipe, ov);
      break_session(s);
      return NIPC_NP_ERR_SEND;
    }
  }
}

/* Write one packet with the session's handle kind. A client with
 * progress_bound set (more than one request in flight) reads what the server
 * sent into the pending queue until its write completes, so both sides keep
 * making progress. Without a wait it waits as long as a blocking write
 * would; with one it also ends at the deadline or abort. */
static nipc_np_error_t pipe_send(nipc_np_session_t *s, const void *data,
                                 size_t len, const receive_wait_t *wait) {
  if (!s->write_event)
    return raw_send(s->pipe, data, len);

  OVERLAPPED ov = {0};
  ov.hEvent = s->write_event;
  nipc_np_error_t err = start_overlapped_send(s->pipe, &ov, data, len);
  if (err != NIPC_NP_OK)
    return err;
  if (s->progress_bound == 0)
    return wait ? finish_overlapped_send_wait(s, &ov, len, wait)
                : finish_overlapped_send(s->pipe, &ov, len);

  bool eof = false;
  ULONGLONG yield_until = progress_now_us() + NP_PROGRESS_YIELD_US;
  for (;;) {
    DWORD w = WaitForSingleObject(s->write_event, 0);
    if (w == WAIT_OBJECT_0)
      return finish_overlapped_send(s->pipe, &ov, len);
    if (w != WAIT_TIMEOUT) {
      cancel_overlapped_send(s->pipe, &ov);
      break_session(s);
      return NIPC_NP_ERR_SEND;
    }
    if (wait) {
      err = send_wait_ended(s, &ov, wait);
      if (err != NIPC_NP_OK)
        return err;
    }
    if (!eof) {
      bool drained = false;
      err = pending_drain(s, &drained, &eof);
      if (err != NIPC_NP_OK) {
        cancel_overlapped_send(s->pipe, &ov);
        break_session(s);
        return err;
      }
      if (drained) {
        yield_until = progress_now_us() + NP_PROGRESS_YIELD_US;
        continue;
      }
    }
    /* The server is busy or its reply is still being written: yield for up
     * to NP_PROGRESS_YIELD_US after the last progress, then wait for the
     * write in short slices. A slice lasts a system timer tick and only the
     * write completing ends it early, so slicing at once stalls a server
     * blocked on its reply to us. */
    if (progress_now_us() < yield_until) {
      SwitchToThread();
      continue;
    }
    WaitForSingleObject(s->write_event, 1);
  }
}

/* Poll a synchronous pipe handle until bytes are available. Go/Rust already
 * use PeekNamedPipe here; waiting on the pipe HANDLE itself is not reliable
 * for request readability. */
static nipc_np_error_t wait_readable(HANDLE pipe, uint32_t timeout_ms,
                                     bool *readable_out) {
  ULONGLONG deadline = GetTickCount64() + timeout_ms;
  int yielded = 0;

  if (readable_out)
    *readable_out = false;

  for (;;) {
    DWORD available = 0;
    BOOL ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
    if (!ok) {
      DWORD err = GetLastError();
      if (is_disconnect_error(err))
        return NIPC_NP_ERR_DISCONNECTED;
      return NIPC_NP_ERR_RECV;
    }

    if (available > 0) {
      if (readable_out)
        *readable_out = true;
      return NIPC_NP_OK;
    }

    if (GetTickCount64() >= deadline)
      return NIPC_NP_OK;

    if (!yielded) {
      yielded = 1;
      for (int i = 0; i < 256; i++) {
        SwitchToThread();

        available = 0;
        ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
        if (!ok) {
          DWORD err = GetLastError();
          if (is_disconnect_error(err))
            return NIPC_NP_ERR_DISCONNECTED;
          return NIPC_NP_ERR_RECV;
        }

        if (available > 0) {
          if (readable_out)
            *readable_out = true;
          return NIPC_NP_OK;
        }

        if (GetTickCount64() >= deadline)
          return NIPC_NP_OK;
      }
      continue;
    }

    Sleep(1);
  }
}

static nipc_np_error_t wait_readable_for_receive(HANDLE pipe,
                                                 const receive_wait_t *wait,
                                                 bool *readable_out) {
  int yielded = 0;

  if (readable_out)
    *readable_out = false;

  for (;;) {
    if (receive_wait_aborted(wait))
      return NIPC_NP_ERR_ABORTED;

    DWORD available = 0;
    BOOL ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
    if (!ok) {
      DWORD err = GetLastError();
      if (is_disconnect_error(err))
        return NIPC_NP_ERR_DISCONNECTED;
      return NIPC_NP_ERR_RECV;
    }

    if (available > 0) {
      if (readable_out)
        *readable_out = true;
      return NIPC_NP_OK;
    }

    if (receive_wait_expired(wait))
      return NIPC_NP_ERR_TIMEOUT;

    if (!yielded) {
      yielded = 1;
      for (int i = 0; i < 256; i++) {
        if (receive_wait_aborted(wait))
          return NIPC_NP_ERR_ABORTED;

        SwitchToThread();

        available = 0;
        ok = PeekNamedPipe(pipe, NULL, 0, NULL, &available, NULL);
        if (!ok) {
          DWORD err = GetLastError();
          if (is_disconnect_error(err))
            return NIPC_NP_ERR_DISCONNECTED;
          return NIPC_NP_ERR_RECV;
        }

        if (available > 0) {
          if (readable_out)
            *readable_out = true;
          return NIPC_NP_OK;
        }

        if (receive_wait_expired(wait))
          return NIPC_NP_ERR_TIMEOUT;
      }
      continue;
    }

    Sleep(1);
  }
}

static nipc_np_error_t raw_recv_with_wait(nipc_np_session_t *session, void *buf,
                                          size_t buf_size,
                                          const receive_wait_t *wait,
                                          DWORD *bytes_read) {
  if (nipc_np_session_has_pending(session)) {
    if (pending_pop(session, buf, buf_size, bytes_read) <= 0) {
      inflight_fail_all(session);
      return NIPC_NP_ERR_RECV;
    }
    return NIPC_NP_OK;
  }
  bool readable = false;
  nipc_np_error_t werr =
      wait_readable_for_receive(session->pipe, wait, &readable);
  if (werr != NIPC_NP_OK)
    return werr;
  if (!readable)
    return NIPC_NP_ERR_TIMEOUT;

  int rc = pipe_recv(session, buf, buf_size, bytes_read);
  if (rc <= 0) {
    inflight_fail_all(session);
    return NIPC_NP_ERR_RECV;
  }
  return NIPC_NP_OK;
}

static nipc_np_error_t raw_recv_blocking(nipc_np_session_t *session, void *buf,
                                         size_t buf_size, DWORD *bytes_read) {
  int rc = nipc_np_session_has_pending(session)
               ? pending_pop(session, buf, buf_size, bytes_read)
               : pipe_recv(session, buf, buf_size, bytes_read);
  if (rc <= 0) {
    inflight_fail_all(session);
    return NIPC_NP_ERR_RECV;
  }
  return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  In-flight message_id tracking (client side)                        */
/* ------------------------------------------------------------------ */

static int inflight_add(nipc_np_session_t *s, uint64_t id) {
  for (uint32_t i = 0; i < s->inflight_count; i++) {
    if (s->inflight_ids[i] == id)
      return -1; /* duplicate */
  }
  if (s->inflight_count >= s->inflight_capacity) {
    if (s->inflight_capacity > UINT32_MAX / 2)
      return -2; /* allocation failure */
    uint32_t new_cap = s->inflight_capacity ? s->inflight_capacity * 2 : 16;
    if ((size_t)new_cap > SIZE_MAX / sizeof(uint64_t))
      return -2; /* allocation failure */
    uint64_t *new_ids =
        realloc(s->inflight_ids, (size_t)new_cap * sizeof(uint64_t));
    if (!new_ids)
      return -2; /* allocation failure */
    s->inflight_ids = new_ids;
    s->inflight_capacity = new_cap;
  }
  s->inflight_ids[s->inflight_count++] = id;
  return 0;
}

static int inflight_remove(nipc_np_session_t *s, uint64_t id) {
  for (uint32_t i = 0; i < s->inflight_count; i++) {
    if (s->inflight_ids[i] == id) {
      s->inflight_ids[i] = s->inflight_ids[s->inflight_count - 1];
      s->inflight_count--;
      return 0;
    }
  }
  return -1; /* not found */
}

static void inflight_fail_all(nipc_np_session_t *s) {
  if (!s || s->role != NIPC_NP_ROLE_CLIENT)
    return;

  /* A broken session invalidates every in-flight request on it. */
  s->inflight_count = 0;
}

/* ------------------------------------------------------------------ */
/*  Handshake: client side                                             */
/* ------------------------------------------------------------------ */

static nipc_np_error_t client_handshake(HANDLE pipe,
                                        const nipc_np_client_config_t *cfg,
                                        const receive_wait_t *wait,
                                        nipc_np_session_t *session) {
  uint8_t buf[128];

  uint32_t pkt_size =
      apply_default(cfg->packet_size, NIPC_NP_DEFAULT_PACKET_SIZE);

  /* Build HELLO payload */
  nipc_hello_t hello = {
      .layout_version = 1,
      .flags = 0,
      .supported_profiles = (cfg->supported_profiles ? cfg->supported_profiles
                                                     : NIPC_PROFILE_BASELINE) &
                            NIPC_NP_PLATFORM_PROFILES,
      .preferred_profiles = cfg->preferred_profiles & NIPC_NP_PLATFORM_PROFILES,
      .max_request_payload_bytes = apply_default(cfg->max_request_payload_bytes,
                                                 NIPC_MAX_PAYLOAD_DEFAULT),
      .max_request_batch_items = apply_default(cfg->max_request_batch_items,
                                               NIPC_NP_DEFAULT_BATCH_ITEMS),
      .max_response_payload_bytes = apply_default(
          cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT),
      .max_response_batch_items = apply_default(cfg->max_response_batch_items,
                                                NIPC_NP_DEFAULT_BATCH_ITEMS),
      .auth_token = cfg->auth_token,
      .packet_size = pkt_size,
  };

  uint8_t hello_buf[44];
  nipc_hello_encode(&hello, hello_buf, sizeof(hello_buf));

  /* Build outer CONTROL header */
  nipc_header_t hdr = {
      .magic = NIPC_MAGIC_MSG,
      .version = NIPC_VERSION,
      .header_len = NIPC_HEADER_LEN,
      .kind = NIPC_KIND_CONTROL,
      .flags = 0,
      .code = NIPC_CODE_HELLO,
      .transport_status = NIPC_STATUS_OK,
      .payload_len = sizeof(hello_buf),
      .item_count = 1,
      .message_id = 0,
  };

  nipc_header_encode(&hdr, buf, sizeof(buf));
  memcpy(buf + NIPC_HEADER_LEN, hello_buf, sizeof(hello_buf));

  /* Send HELLO */
  OVERLAPPED ov = {0};
  ov.hEvent = session->write_event;
  nipc_np_error_t err = start_overlapped_send(pipe, &ov, buf,
                                              NIPC_HEADER_LEN + sizeof(hello_buf));
  if (err == NIPC_NP_OK)
    err = finish_overlapped_send(pipe, &ov, NIPC_HEADER_LEN + sizeof(hello_buf));
  if (err != NIPC_NP_OK)
    return err;

  /* Receive HELLO_ACK; a server that never answers must not hold the
   * client past its deadline. A server that rejects the client writes its
   * reply and closes, so a disconnected pipe may still hold that reply. */
  err = wait_readable_for_receive(pipe, wait, NULL);
  if (err != NIPC_NP_OK && err != NIPC_NP_ERR_DISCONNECTED)
    return err;

  DWORD bytes_read = 0;
  int rc = overlapped_recv(pipe, session->read_event, buf, sizeof(buf),
                           &bytes_read);
  if (rc <= 0)
    return NIPC_NP_ERR_RECV;

  /* Decode outer header */
  nipc_header_t ack_hdr;
  nipc_error_t perr = nipc_header_decode(buf, (size_t)bytes_read, &ack_hdr);
  if (perr == NIPC_ERR_BAD_VERSION)
    return NIPC_NP_ERR_INCOMPATIBLE;
  if (perr != NIPC_OK)
    return NIPC_NP_ERR_PROTOCOL;

  if (ack_hdr.kind != NIPC_KIND_CONTROL || ack_hdr.code != NIPC_CODE_HELLO_ACK)
    return NIPC_NP_ERR_PROTOCOL;

  /* Check transport_status for rejection */
  if (ack_hdr.transport_status == NIPC_STATUS_AUTH_FAILED)
    return NIPC_NP_ERR_AUTH_FAILED;
  if (ack_hdr.transport_status == NIPC_STATUS_UNSUPPORTED)
    return NIPC_NP_ERR_NO_PROFILE;
  if (ack_hdr.transport_status == NIPC_STATUS_INCOMPATIBLE)
    return NIPC_NP_ERR_INCOMPATIBLE;
  if (ack_hdr.transport_status == NIPC_STATUS_LIMIT_EXCEEDED)
    return NIPC_NP_ERR_LIMIT_EXCEEDED;
  if (ack_hdr.transport_status != NIPC_STATUS_OK)
    return NIPC_NP_ERR_HANDSHAKE;

  /* Decode hello-ack payload */
  nipc_hello_ack_t ack;
  perr = nipc_hello_ack_decode(buf + NIPC_HEADER_LEN,
                               (size_t)bytes_read - NIPC_HEADER_LEN, &ack);
  if (perr == NIPC_ERR_BAD_LAYOUT &&
      hello_ack_layout_incompatible(buf + NIPC_HEADER_LEN,
                                    (size_t)bytes_read - NIPC_HEADER_LEN))
    return NIPC_NP_ERR_INCOMPATIBLE;
  if (perr != NIPC_OK)
    return NIPC_NP_ERR_PROTOCOL;
  /* HELLO_ACK is exactly one 48-byte payload; extra bytes would otherwise
   * be ignored. */
  if (ack_hdr.payload_len != sizeof(nipc_hello_ack_t) ||
      (size_t)bytes_read != NIPC_HEADER_LEN + sizeof(nipc_hello_ack_t))
    return NIPC_NP_ERR_PROTOCOL;

  /* The server must pick exactly one of the profiles this client offered. */
  if (ack.selected_profile == 0 ||
      (ack.selected_profile & (ack.selected_profile - 1)) != 0 ||
      (ack.selected_profile & hello.supported_profiles) == 0)
    return NIPC_NP_ERR_PROTOCOL;

  /* The agreed packet size is at most what this client proposed, and must
   * leave room for a payload after the header. */
  if (ack.agreed_packet_size <= NIPC_HEADER_LEN ||
      ack.agreed_packet_size > pkt_size)
    return NIPC_NP_ERR_PROTOCOL;

  /* Fill session */
  session->pipe = pipe;
  session->role = NIPC_NP_ROLE_CLIENT;
  session->max_request_payload_bytes = ack.agreed_max_request_payload_bytes;
  session->max_request_batch_items = ack.agreed_max_request_batch_items;
  session->max_response_payload_bytes = ack.agreed_max_response_payload_bytes;
  session->max_response_batch_items = ack.agreed_max_response_batch_items;
  session->packet_size = ack.agreed_packet_size;
  session->selected_profile = ack.selected_profile;
  session->session_id = ack.session_id;
  session->recv_buf = NULL;
  session->recv_buf_size = 0;
  session->inflight_count = 0;

  return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Handshake: server side                                             */
/* ------------------------------------------------------------------ */

static nipc_np_error_t server_handshake(HANDLE pipe,
                                        const nipc_np_server_config_t *cfg,
                                        uint64_t session_id,
                                        nipc_np_profile_prepare_fn prepare,
                                        void *prepare_user,
                                        nipc_np_session_t *session) {
  uint8_t buf[128];

  uint32_t server_pkt_size =
      apply_default(cfg->packet_size, NIPC_NP_DEFAULT_PACKET_SIZE);
  uint32_t s_req_pay =
      apply_default(cfg->max_request_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
  uint32_t s_resp_pay =
      apply_default(cfg->max_response_payload_bytes, NIPC_MAX_PAYLOAD_DEFAULT);
  uint32_t s_profiles =
      (cfg->supported_profiles ? cfg->supported_profiles : NIPC_PROFILE_BASELINE) &
      NIPC_NP_PLATFORM_PROFILES;
  uint32_t s_preferred = cfg->preferred_profiles & NIPC_NP_PLATFORM_PROFILES;

  /* Receive HELLO */
  DWORD bytes_read = 0;
  int rc = raw_recv(pipe, buf, sizeof(buf), &bytes_read);
  if (rc <= 0)
    return NIPC_NP_ERR_RECV;

  nipc_header_t hdr;
  nipc_error_t perr = nipc_header_decode(buf, (size_t)bytes_read, &hdr);
  if (perr == NIPC_ERR_BAD_VERSION &&
      header_version_incompatible(buf, (size_t)bytes_read, NIPC_CODE_HELLO)) {
    send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_NP_ERR_INCOMPATIBLE;
  }
  if (perr != NIPC_OK)
    return NIPC_NP_ERR_PROTOCOL;

  if (hdr.kind != NIPC_KIND_CONTROL || hdr.code != NIPC_CODE_HELLO)
    return NIPC_NP_ERR_PROTOCOL;

  nipc_hello_t hello;
  perr = nipc_hello_decode(buf + NIPC_HEADER_LEN,
                           (size_t)bytes_read - NIPC_HEADER_LEN, &hello);
  if (perr == NIPC_ERR_BAD_LAYOUT &&
      hello_layout_incompatible(buf + NIPC_HEADER_LEN,
                                (size_t)bytes_read - NIPC_HEADER_LEN)) {
    send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_NP_ERR_INCOMPATIBLE;
  }
  if (perr != NIPC_OK)
    return NIPC_NP_ERR_PROTOCOL;
  /* HELLO is exactly one 44-byte payload, as HELLO_ACK is 48. */
  if (hdr.payload_len != NIPC_HELLO_WIRE_SIZE ||
      (size_t)bytes_read != NIPC_HEADER_LEN + NIPC_HELLO_WIRE_SIZE)
    return NIPC_NP_ERR_PROTOCOL;

  /* Compute intersection */
  uint32_t intersection = hello.supported_profiles & s_profiles;

  /* Check intersection */
  if (intersection == 0) {
    send_rejection_ack(pipe, NIPC_STATUS_UNSUPPORTED);
    return NIPC_NP_ERR_NO_PROFILE;
  }

  /* Check auth */
  if (hello.auth_token != cfg->auth_token) {
    send_rejection_ack(pipe, NIPC_STATUS_AUTH_FAILED);
    return NIPC_NP_ERR_AUTH_FAILED;
  }

  /* Select profile */
  uint32_t preferred_intersection =
      intersection & hello.preferred_profiles & s_preferred;
  uint32_t selected;
  if (preferred_intersection != 0)
    selected = highest_bit(preferred_intersection);
  else
    selected = highest_bit(intersection);

  if (hello.max_request_payload_bytes > s_req_pay) {
    send_rejection_ack(pipe, NIPC_STATUS_LIMIT_EXCEEDED);
    return NIPC_NP_ERR_LIMIT_EXCEEDED;
  }

  /* Negotiate limits:
   * - request payload and batch size are client-proposed and echoed
   * - response payload is server-authoritative
   * - response batch size is symmetric with request batch size */
  uint32_t agreed_req_pay = hello.max_request_payload_bytes;
  uint32_t agreed_req_bat = hello.max_request_batch_items;
  uint32_t agreed_resp_pay = s_resp_pay;
  uint32_t agreed_resp_bat = agreed_req_bat;
  uint32_t agreed_pkt = min_u32(hello.packet_size, server_pkt_size);

  /* Reject packet sizes too small for a usable message packet */
  if (agreed_pkt <= NIPC_HEADER_LEN) {
    send_rejection_ack(pipe, NIPC_STATUS_INCOMPATIBLE);
    return NIPC_NP_ERR_INCOMPATIBLE;
  }

  /* Set up the selected profile before the ACK promises it. A profile the
   * caller cannot prepare is dropped, as if the server never offered it. */
  while (prepare &&
         !prepare(prepare_user, selected, agreed_req_pay, agreed_resp_pay)) {
    s_profiles &= ~selected;
    intersection &= ~selected;
    if (intersection == 0) {
      send_rejection_ack(pipe, NIPC_STATUS_UNSUPPORTED);
      return NIPC_NP_ERR_NO_PROFILE;
    }
    preferred_intersection = intersection & hello.preferred_profiles & s_preferred;
    selected = preferred_intersection != 0 ? highest_bit(preferred_intersection)
                                           : highest_bit(intersection);
  }

  /* Send HELLO_ACK (success) */
  nipc_hello_ack_t ack = {
      .layout_version = 1,
      .flags = 0,
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

  nipc_header_t ack_hdr = {
      .magic = NIPC_MAGIC_MSG,
      .version = NIPC_VERSION,
      .header_len = NIPC_HEADER_LEN,
      .kind = NIPC_KIND_CONTROL,
      .flags = 0,
      .code = NIPC_CODE_HELLO_ACK,
      .transport_status = NIPC_STATUS_OK,
      .payload_len = sizeof(ack_buf),
      .item_count = 1,
      .message_id = 0,
  };

  uint8_t pkt[80];
  nipc_header_encode(&ack_hdr, pkt, sizeof(pkt));
  memcpy(pkt + NIPC_HEADER_LEN, ack_buf, sizeof(ack_buf));

  nipc_np_error_t send_err =
      raw_send(pipe, pkt, NIPC_HEADER_LEN + sizeof(ack_buf));
  if (send_err != NIPC_NP_OK)
    return send_err;

  /* Fill session */
  session->pipe = pipe;
  session->role = NIPC_NP_ROLE_SERVER;
  session->max_request_payload_bytes = agreed_req_pay;
  session->max_request_batch_items = agreed_req_bat;
  session->max_response_payload_bytes = agreed_resp_pay;
  session->max_response_batch_items = agreed_resp_bat;
  session->packet_size = agreed_pkt;
  session->selected_profile = selected;
  session->session_id = session_id;
  session->recv_buf = NULL;
  session->recv_buf_size = 0;
  session->inflight_count = 0;

  return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: listen                                                 */
/* ------------------------------------------------------------------ */

nipc_np_error_t nipc_np_listen(const char *run_dir, const char *service_name,
                               const nipc_np_server_config_t *config,
                               nipc_np_listener_t *out) {
  if (!config || !out)
    return NIPC_NP_ERR_BAD_PARAM;

  memset(out, 0, sizeof(*out));
  out->pipe = INVALID_HANDLE_VALUE;

  /* Build pipe name */
  if (nipc_np_build_pipe_name(out->pipe_name, NIPC_NP_MAX_PIPE_NAME, run_dir,
                              service_name) < 0)
    return NIPC_NP_ERR_BAD_PARAM;

  /* Determine buffer size */
  uint32_t buf_size = pipe_buffer_size(config->packet_size);

  /* Create first pipe instance with FILE_FLAG_FIRST_PIPE_INSTANCE
   * to detect if another server already owns this pipe name. */
  HANDLE h = create_pipe_instance(out->pipe_name, buf_size, TRUE);
  if (h == INVALID_HANDLE_VALUE) {
    DWORD err = GetLastError();
    if (err == ERROR_ACCESS_DENIED || err == ERROR_PIPE_BUSY)
      return NIPC_NP_ERR_ADDR_IN_USE;
    return NIPC_NP_ERR_CREATE_PIPE;
  }

  out->pipe = h;
  out->config = *config;

  return NIPC_NP_OK;
}

/* ------------------------------------------------------------------ */
/*  Public API: accept                                                 */
/* ------------------------------------------------------------------ */

static volatile LONG g_fail_next_instances;

void nipc_np_fail_next_instances_for_tests(int count) {
  InterlockedExchange(&g_fail_next_instances, count);
}

static bool fail_next_instance(void) {
  for (;;) {
    LONG left = g_fail_next_instances;
    if (left <= 0)
      return false;
    if (InterlockedCompareExchange(&g_fail_next_instances, left - 1, left) ==
        left)
      return true;
  }
}

nipc_np_error_t nipc_np_accept_pending(nipc_np_listener_t *listener,
                                       HANDLE *pipe_out) {
  if (!listener || !pipe_out)
    return NIPC_NP_ERR_BAD_PARAM;
  *pipe_out = INVALID_HANDLE_VALUE;
  if (listener->pipe == INVALID_HANDLE_VALUE || listener->pipe == NULL)
    return NIPC_NP_ERR_ACCEPT;

  /* Wait for client to connect to the current pipe instance */
  BOOL connected = ConnectNamedPipe(listener->pipe, NULL);
  if (!connected) {
    DWORD err = GetLastError();
    /* ERROR_PIPE_CONNECTED means a client connected between
     * CreateNamedPipe and ConnectNamedPipe — that's fine. */
    if (err != ERROR_PIPE_CONNECTED) {
      return NIPC_NP_ERR_ACCEPT;
    }
  }

  /* The listener's current pipe instance is now the session pipe. */
  HANDLE session_pipe = listener->pipe;

  /* Create a new pipe instance for the next client. */
  uint32_t buf_size = pipe_buffer_size(listener->config.packet_size);
  HANDLE next = fail_next_instance()
                    ? INVALID_HANDLE_VALUE
                    : create_pipe_instance(listener->pipe_name, buf_size, FALSE);
  if (next == INVALID_HANDLE_VALUE) {
    /* Without a next instance, drop this client and keep its instance as
     * the listener's, so a later accept listens on it again. */
    DisconnectNamedPipe(session_pipe);
    return NIPC_NP_ERR_CREATE_PIPE;
  }
  listener->pipe = next;

  *pipe_out = session_pipe;
  return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_accept_handshake(HANDLE pipe,
                                         const nipc_np_server_config_t *config,
                                         uint64_t session_id,
                                         uint32_t timeout_ms,
                                         nipc_np_session_t *out) {
  return nipc_np_accept_handshake_prepared(pipe, config, session_id,
                                           timeout_ms, NULL, NULL, out);
}

nipc_np_error_t nipc_np_accept_handshake_prepared(
    HANDLE pipe, const nipc_np_server_config_t *config, uint64_t session_id,
    uint32_t timeout_ms, nipc_np_profile_prepare_fn prepare, void *user,
    nipc_np_session_t *out) {
  if (!out || !config || pipe == INVALID_HANDLE_VALUE || pipe == NULL) {
    if (pipe != INVALID_HANDLE_VALUE && pipe != NULL)
      CloseHandle(pipe);
    return NIPC_NP_ERR_BAD_PARAM;
  }

  memset(out, 0, sizeof(*out));
  out->pipe = INVALID_HANDLE_VALUE;

  bool readable = false;
  nipc_np_error_t herr;
  do {
    herr = wait_readable(pipe, timeout_ms ? timeout_ms : 1000u, &readable);
  } while (herr == NIPC_NP_OK && !readable && timeout_ms == 0);
  if (herr == NIPC_NP_OK && !readable)
    herr = NIPC_NP_ERR_TIMEOUT;
  /* A peer gone before HELLO is a failed HELLO read, as without the wait. */
  if (herr == NIPC_NP_ERR_DISCONNECTED)
    herr = NIPC_NP_ERR_RECV;
  if (herr == NIPC_NP_OK)
    herr = server_handshake(pipe, config, session_id, prepare, user, out);
  if (herr != NIPC_NP_OK) {
    /* Close only: DisconnectNamedPipe would discard a rejection reply the
     * client has not read yet. */
    CloseHandle(pipe);
    out->pipe = INVALID_HANDLE_VALUE;
    return herr;
  }

  return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_accept(nipc_np_listener_t *listener,
                               uint64_t session_id, nipc_np_session_t *out) {
  if (!listener || !out)
    return NIPC_NP_ERR_BAD_PARAM;

  memset(out, 0, sizeof(*out));
  out->pipe = INVALID_HANDLE_VALUE;

  HANDLE session_pipe;
  nipc_np_error_t err = nipc_np_accept_pending(listener, &session_pipe);
  if (err != NIPC_NP_OK)
    return err;

  return nipc_np_accept_handshake(session_pipe, &listener->config, session_id,
                                  NIPC_SERVER_HANDSHAKE_TIMEOUT_MS, out);
}

/* ------------------------------------------------------------------ */
/*  Public API: connect                                                */
/* ------------------------------------------------------------------ */

static void (*g_connect_busy_hook)(void *user);
static void *g_connect_busy_user;

void nipc_np_set_connect_busy_hook_for_tests(void (*hook)(void *user),
                                             void *user) {
  g_connect_busy_hook = hook;
  g_connect_busy_user = user;
}

static void (*g_connect_waited_hook)(void *user);
static void *g_connect_waited_user;

void nipc_np_set_connect_waited_hook_for_tests(void (*hook)(void *user),
                                               void *user) {
  g_connect_waited_hook = hook;
  g_connect_waited_user = user;
}

/* SECURITY_IDENTIFICATION lets the server identify the client but never act
 * as it. Overlapped, so a pipelining send can read while its write waits. */
static HANDLE create_client_pipe(const wchar_t *pipe_name) {
  return CreateFileW(pipe_name, GENERIC_READ | GENERIC_WRITE, 0, /* no sharing */
                     NULL, /* default security */
                     OPEN_EXISTING,
                     SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION |
                         FILE_FLAG_OVERLAPPED,
                     NULL /* no template */
  );
}

/* Longest single busy-pipe wait, so an abort ends it promptly. */
#define NP_CONNECT_BUSY_SLICE_MS 100u

/* A server has no free pipe instance between accepting one client and
 * creating the next instance, and connects in that window fail with
 * ERROR_PIPE_BUSY. WaitNamedPipeW does not reserve the instance it reports,
 * so another client can take it first: wait and retry, bounded by
 * NIPC_NP_CONNECT_BUSY_WAIT_MS and the caller's deadline. An abort or an
 * expired deadline ends the connect before the pipe is opened. */
static HANDLE open_client_pipe(const wchar_t *pipe_name,
                               const receive_wait_t *wait,
                               nipc_np_error_t *err_out) {
  *err_out = NIPC_NP_ERR_CONNECT;
  if (receive_wait_aborted(wait)) {
    *err_out = NIPC_NP_ERR_ABORTED;
    return INVALID_HANDLE_VALUE;
  }
  if (receive_wait_expired(wait)) {
    *err_out = NIPC_NP_ERR_TIMEOUT;
    return INVALID_HANDLE_VALUE;
  }
  HANDLE h = create_client_pipe(pipe_name);
  if (h != INVALID_HANDLE_VALUE || GetLastError() != ERROR_PIPE_BUSY)
    return h;

  if (g_connect_busy_hook)
    g_connect_busy_hook(g_connect_busy_user);

  ULONGLONG busy_deadline = GetTickCount64() + NIPC_NP_CONNECT_BUSY_WAIT_MS;
  if (!wait->infinite && wait->deadline < busy_deadline)
    busy_deadline = wait->deadline;
  for (;;) {
    if (receive_wait_aborted(wait)) {
      *err_out = NIPC_NP_ERR_ABORTED;
      return INVALID_HANDLE_VALUE;
    }
    ULONGLONG now = GetTickCount64();
    if (now >= busy_deadline) {
      if (receive_wait_expired(wait))
        *err_out = NIPC_NP_ERR_TIMEOUT;
      return INVALID_HANDLE_VALUE;
    }
    ULONGLONG left = busy_deadline - now;
    DWORD slice = left < NP_CONNECT_BUSY_SLICE_MS ? (DWORD)left
                                                  : NP_CONNECT_BUSY_SLICE_MS;
    if (!WaitNamedPipeW(pipe_name, slice)) {
      if (GetLastError() != ERROR_SEM_TIMEOUT)
        return INVALID_HANDLE_VALUE;
      continue;
    }
    if (g_connect_waited_hook)
      g_connect_waited_hook(g_connect_waited_user);
    h = create_client_pipe(pipe_name);
    if (h != INVALID_HANDLE_VALUE || GetLastError() != ERROR_PIPE_BUSY)
      return h;
    /* Another client took the instance first. */
  }
}

nipc_np_error_t nipc_np_connect_timeout(const char *run_dir,
                                        const char *service_name,
                                        const nipc_np_client_config_t *config,
                                        uint32_t timeout_ms,
                                        HANDLE abort_event,
                                        nipc_np_session_t *out) {
  if (!config || !out)
    return NIPC_NP_ERR_BAD_PARAM;

  memset(out, 0, sizeof(*out));
  out->pipe = INVALID_HANDLE_VALUE;

  receive_wait_t wait = receive_wait_init(timeout_ms, abort_event);

  /* Build pipe name */
  wchar_t pipe_name[NIPC_NP_MAX_PIPE_NAME];
  if (nipc_np_build_pipe_name(pipe_name, NIPC_NP_MAX_PIPE_NAME, run_dir,
                              service_name) < 0)
    return NIPC_NP_ERR_BAD_PARAM;

  /* Connect to the server pipe */
  nipc_np_error_t err;
  HANDLE h = open_client_pipe(pipe_name, &wait, &err);
  if (h == INVALID_HANDLE_VALUE)
    return err;

  /* Whoever created the pipe name first owns it. Send HELLO and the auth
   * token only to a server running as this account, SYSTEM or Administrators;
   * other accounts cannot take those as owner. */
  if (!nipc_win_owner_trusted(h)) {
    CloseHandle(h);
    return NIPC_NP_ERR_UNTRUSTED_SERVER;
  }

  /* Set read mode to message mode */
  DWORD mode = PIPE_READMODE_MESSAGE;
  if (!SetNamedPipeHandleState(h, &mode, NULL, NULL)) {
    CloseHandle(h);
    return NIPC_NP_ERR_CONNECT;
  }

  out->read_event = CreateEventW(NULL, TRUE, FALSE, NULL);
  out->write_event = CreateEventW(NULL, TRUE, FALSE, NULL);
  if (!out->read_event || !out->write_event) {
    CloseHandle(h);
    nipc_np_close_session(out);
    return NIPC_NP_ERR_CONNECT;
  }

  /* Perform handshake */
  err = client_handshake(h, config, &wait, out);
  if (err != NIPC_NP_OK) {
    CloseHandle(h);
    out->pipe = INVALID_HANDLE_VALUE;
    nipc_np_close_session(out);
    return err;
  }

  return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_connect(const char *run_dir, const char *service_name,
                                const nipc_np_client_config_t *config,
                                nipc_np_session_t *out) {
  return nipc_np_connect_timeout(run_dir, service_name, config, 0, NULL, out);
}

/* ------------------------------------------------------------------ */
/*  Public API: close                                                  */
/* ------------------------------------------------------------------ */

void nipc_np_close_session(nipc_np_session_t *session) {
  if (!session)
    return;

  if (session->pipe != INVALID_HANDLE_VALUE && session->pipe != NULL) {
    /* Close only. The peer still reads every message written before the
     * close, then sees ERROR_BROKEN_PIPE. FlushFileBuffers would block on a
     * peer that stopped reading, and neither CancelSynchronousIo nor
     * DisconnectNamedPipe from another thread releases it. */
    CloseHandle(session->pipe);
  }
  session->pipe = INVALID_HANDLE_VALUE;

  free(session->recv_buf);
  session->recv_buf = NULL;
  session->recv_buf_size = 0;

  free(session->inflight_ids);
  session->inflight_ids = NULL;
  session->inflight_count = 0;
  session->inflight_capacity = 0;

  /* No I/O stays posted between calls, so the events can go. */
  if (session->read_event)
    CloseHandle(session->read_event);
  if (session->write_event)
    CloseHandle(session->write_event);
  session->read_event = NULL;
  session->write_event = NULL;
  free(session->pending_buf);
  session->pending_buf = NULL;
  session->pending_len = session->pending_head = session->pending_cap = 0;
  session->progress_bound = 0;
}

void nipc_np_close_listener(nipc_np_listener_t *listener) {
  if (!listener)
    return;

  if (listener->pipe != INVALID_HANDLE_VALUE && listener->pipe != NULL) {
    /* A loopback connect reliably wakes a blocking ConnectNamedPipe()
     * so the accept thread can observe shutdown and exit. */
    if (listener->pipe_name[0] != L'\0') {
      HANDLE wake =
          CreateFileW(listener->pipe_name, GENERIC_READ | GENERIC_WRITE, 0,
                      NULL, OPEN_EXISTING,
                      SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, NULL);
      if (wake != INVALID_HANDLE_VALUE)
        CloseHandle(wake);
    }
    CloseHandle(listener->pipe);
  }
  listener->pipe = INVALID_HANDLE_VALUE;
}

/* ------------------------------------------------------------------ */
/*  Public API: send                                                   */
/* ------------------------------------------------------------------ */

static nipc_np_error_t np_send(nipc_np_session_t *session, nipc_header_t *hdr,
                               const void *payload, size_t payload_len,
                               const receive_wait_t *wait) {
  if (!session || session->pipe == INVALID_HANDLE_VALUE)
    return NIPC_NP_ERR_BAD_PARAM;

  int tracked =
      (session->role == NIPC_NP_ROLE_CLIENT && hdr->kind == NIPC_KIND_REQUEST);

  /* Client-side: track in-flight message_ids for requests */
  if (tracked) {
    int rc = inflight_add(session, hdr->message_id);
    if (rc == -1)
      return NIPC_NP_ERR_DUPLICATE_MSG_ID;
    if (rc == -2)
      return NIPC_NP_ERR_LIMIT_EXCEEDED;
  }

  /* One request in flight (every typed call) writes without draining; only a
   * pipelining client can deadlock with a server blocked writing to it. */
  session->progress_bound = 0;
  if (session->write_event && session->inflight_count > 1)
    session->progress_bound =
        pending_bound(session->max_response_payload_bytes,
                      session->packet_size, session->inflight_count);

  uint32_t max_payload = 0;
  uint32_t max_batch = 0;
  if (session->role == NIPC_NP_ROLE_CLIENT && hdr->kind == NIPC_KIND_REQUEST) {
    max_payload = session->max_request_payload_bytes;
    max_batch = session->max_request_batch_items;
  } else if (session->role == NIPC_NP_ROLE_SERVER &&
             hdr->kind == NIPC_KIND_RESPONSE) {
    max_payload = session->max_response_payload_bytes;
    max_batch = session->max_response_batch_items;
  }
  if (payload_len > UINT32_MAX ||
      (max_payload > 0 && payload_len > max_payload) ||
      (max_batch > 0 && hdr->item_count > max_batch)) {
    if (tracked)
      inflight_remove(session, hdr->message_id);
    return NIPC_NP_ERR_LIMIT_EXCEEDED;
  }

  /* Fill envelope fields */
  hdr->magic = NIPC_MAGIC_MSG;
  hdr->version = NIPC_VERSION;
  hdr->header_len = NIPC_HEADER_LEN;
  /* validate_outbound_limits() rejects payloads that cannot fit the header. */
  assert(payload_len <= UINT32_MAX);
  hdr->payload_len = (uint32_t)payload_len;

  size_t total_msg;
  if (!header_payload_len(payload_len, &total_msg)) {
    if (tracked)
      inflight_remove(session, hdr->message_id);
    return NIPC_NP_ERR_LIMIT_EXCEEDED;
  }

  /* Single packet? */
  if (total_msg <= session->packet_size) {
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));
    nipc_np_error_t err = raw_send_msg(session, hdr_buf, NIPC_HEADER_LEN,
                                       payload, payload_len, wait);
    if (err != NIPC_NP_OK && tracked) {
      if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
        inflight_fail_all(session);
      else
        inflight_remove(session, hdr->message_id);
    }
    return err;
  }

  /* Chunked send */
  if (session->packet_size <= NIPC_HEADER_LEN) {
    if (tracked)
      inflight_remove(session, hdr->message_id);
    return NIPC_NP_ERR_BAD_PARAM;
  }

  /* header_payload_len() rejects totals wider than the wire field. */
  assert(total_msg <= UINT32_MAX);
  uint32_t total_msg_u32 = (uint32_t)total_msg;

  size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;
  size_t remaining = payload_len;
  size_t first_chunk_payload =
      remaining < chunk_payload_budget ? remaining : chunk_payload_budget;

  remaining -= first_chunk_payload;
  uint32_t continuation_chunks = 0;
  if (remaining > 0) {
    continuation_chunks =
        (uint32_t)(1 + ((remaining - 1) / chunk_payload_budget));
  }
  uint32_t chunk_count = 1 + continuation_chunks;

  /* Send first chunk */
  uint8_t hdr_buf[NIPC_HEADER_LEN];
  nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

  nipc_np_error_t err = raw_send_msg(session, hdr_buf, NIPC_HEADER_LEN,
                                     payload, first_chunk_payload, wait);
  if (err != NIPC_NP_OK) {
    if (tracked) {
      if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
        inflight_fail_all(session);
      else
        inflight_remove(session, hdr->message_id);
    }
    return err;
  }

  /* Send continuation chunks */
  const uint8_t *src = (const uint8_t *)payload + first_chunk_payload;
  remaining = payload_len - first_chunk_payload;

  for (uint32_t ci = 1; ci < chunk_count; ci++) {
    size_t this_chunk =
        remaining < chunk_payload_budget ? remaining : chunk_payload_budget;
    /* packet_size is negotiated as uint32_t, so continuation chunks fit u32. */
    assert(this_chunk <= UINT32_MAX);

    nipc_chunk_header_t chk = {
        .magic = NIPC_MAGIC_CHUNK,
        .version = NIPC_VERSION,
        .flags = 0,
        .message_id = hdr->message_id,
        .total_message_len = total_msg_u32,
        .chunk_index = ci,
        .chunk_count = chunk_count,
        .chunk_payload_len = (uint32_t)this_chunk,
    };

    uint8_t chk_buf[NIPC_HEADER_LEN];
    nipc_chunk_header_encode(&chk, chk_buf, sizeof(chk_buf));

    err =
        raw_send_msg(session, chk_buf, NIPC_HEADER_LEN, src, this_chunk, wait);
    if (err != NIPC_NP_OK) {
      if (tracked) {
        if (err == NIPC_NP_ERR_SEND || err == NIPC_NP_ERR_DISCONNECTED)
          inflight_fail_all(session);
        else
          inflight_remove(session, hdr->message_id);
      }
      return err;
    }

    src += this_chunk;
    remaining -= this_chunk;
  }

  return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_send(nipc_np_session_t *session, nipc_header_t *hdr,
                             const void *payload, size_t payload_len) {
  return np_send(session, hdr, payload, payload_len, NULL);
}

nipc_np_error_t nipc_np_send_timeout(nipc_np_session_t *session,
                                     nipc_header_t *hdr, const void *payload,
                                     size_t payload_len, uint32_t timeout_ms,
                                     HANDLE abort_event) {
  if (timeout_ms == 0 && !abort_event)
    return np_send(session, hdr, payload, payload_len, NULL);
  receive_wait_t wait = receive_wait_init(timeout_ms, abort_event);
  return np_send(session, hdr, payload, payload_len, &wait);
}

/* ------------------------------------------------------------------ */
/*  Public API: receive                                                */
/* ------------------------------------------------------------------ */

static nipc_np_error_t ensure_recv_buf(nipc_np_session_t *session,
                                       size_t needed) {
  if (session->recv_buf_size >= needed)
    return NIPC_NP_OK;

  uint8_t *p = (uint8_t *)realloc(session->recv_buf, needed);
  if (!p)
    return NIPC_NP_ERR_ALLOC;

  session->recv_buf = p;
  session->recv_buf_size = needed;
  return NIPC_NP_OK;
}

/* Validate batch directory if the message has BATCH flag and item_count > 1.
 * Called after the full payload is assembled, before returning to caller. */
static nipc_np_error_t validate_batch(const nipc_header_t *hdr,
                                      const void *payload, size_t payload_len) {
  if (!(hdr->flags & NIPC_FLAG_BATCH) || hdr->item_count <= 1)
    return NIPC_NP_OK;

  if (hdr->item_count > UINT32_MAX / NIPC_LOOKUP_DIR_ENTRY_SIZE)
    return NIPC_NP_ERR_PROTOCOL;
  uint32_t dir_bytes = hdr->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_aligned = nipc_align8((size_t)dir_bytes);
  if (dir_aligned < (size_t)dir_bytes || dir_aligned > UINT32_MAX)
    return NIPC_NP_ERR_PROTOCOL;
  if (payload_len < dir_aligned || payload_len - dir_aligned > UINT32_MAX)
    return NIPC_NP_ERR_PROTOCOL;

  uint32_t packed_area_len = (uint32_t)(payload_len - dir_aligned);
  nipc_error_t perr = nipc_batch_dir_validate(payload, dir_bytes,
                                              hdr->item_count, packed_area_len);
  return (perr == NIPC_OK) ? NIPC_NP_OK : NIPC_NP_ERR_PROTOCOL;
}

nipc_np_error_t nipc_np_receive(nipc_np_session_t *session, void *buf,
                                size_t buf_size, nipc_header_t *hdr_out,
                                const void **payload_out,
                                size_t *payload_len_out) {
  return nipc_np_receive_timeout(session, buf, buf_size, hdr_out, payload_out,
                                 payload_len_out, 0, NULL);
}

nipc_np_error_t nipc_np_receive_timeout(nipc_np_session_t *session, void *buf,
                                        size_t buf_size, nipc_header_t *hdr_out,
                                        const void **payload_out,
                                        size_t *payload_len_out,
                                        uint32_t timeout_ms,
                                        HANDLE abort_event) {
  if (!session || session->pipe == INVALID_HANDLE_VALUE)
    return NIPC_NP_ERR_BAD_PARAM;

  receive_wait_t wait = receive_wait_init(timeout_ms, abort_event);
  bool controlled_receive = timeout_ms != 0 || abort_event != NULL;

  /* Read first message */
  DWORD bytes_read = 0;
  nipc_np_error_t rerr =
      controlled_receive
          ? raw_recv_with_wait(session, buf, buf_size, &wait, &bytes_read)
          : raw_recv_blocking(session, buf, buf_size, &bytes_read);
  if (rerr != NIPC_NP_OK)
    return rerr;

  size_t n = (size_t)bytes_read;
  if (n < NIPC_HEADER_LEN)
    return NIPC_NP_ERR_PROTOCOL;

  /* Decode outer header */
  nipc_error_t perr = nipc_header_decode(buf, n, hdr_out);
  if (perr != NIPC_OK)
    return NIPC_NP_ERR_PROTOCOL;

  /* Validate payload_len against negotiated directional limit */
  uint32_t max_payload = (session->role == NIPC_NP_ROLE_SERVER)
                             ? session->max_request_payload_bytes
                             : session->max_response_payload_bytes;
  if (hdr_out->payload_len > max_payload)
    return NIPC_NP_ERR_LIMIT_EXCEEDED;

  /* Validate item_count */
  uint32_t max_batch = (session->role == NIPC_NP_ROLE_SERVER)
                           ? session->max_request_batch_items
                           : session->max_response_batch_items;
  if (hdr_out->item_count > max_batch)
    return NIPC_NP_ERR_LIMIT_EXCEEDED;

  /* Client-side: validate response message_id is in-flight */
  if (session->role == NIPC_NP_ROLE_CLIENT &&
      hdr_out->kind == NIPC_KIND_RESPONSE) {
    if (inflight_remove(session, hdr_out->message_id) < 0)
      return NIPC_NP_ERR_UNKNOWN_MSG_ID;
  }

  size_t total_msg;
  if (!header_payload_len(hdr_out->payload_len, &total_msg))
    return NIPC_NP_ERR_LIMIT_EXCEEDED;

  if (n > total_msg)
    return NIPC_NP_ERR_PROTOCOL;

  /* Non-chunked: entire message in one read */
  if (n == total_msg) {
    *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
    *payload_len_out = hdr_out->payload_len;

    nipc_np_error_t berr =
        validate_batch(hdr_out, *payload_out, *payload_len_out);
    if (berr != NIPC_NP_OK)
      return berr;

    return NIPC_NP_OK;
  }

  /* Chunked: first message has partial payload */
  size_t first_payload_bytes = n - NIPC_HEADER_LEN;

  nipc_np_error_t err = ensure_recv_buf(session, hdr_out->payload_len);
  if (err != NIPC_NP_OK)
    return err;

  memcpy(session->recv_buf, (uint8_t *)buf + NIPC_HEADER_LEN,
         first_payload_bytes);

  size_t assembled = first_payload_bytes;
  size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;

  /* Calculate expected chunk count */
  size_t remaining_after_first = hdr_out->payload_len - first_payload_bytes;
  uint32_t expected_continuations = 0;
  if (remaining_after_first > 0 && chunk_payload_budget > 0) {
    expected_continuations =
        (uint32_t)(1 + ((remaining_after_first - 1) / chunk_payload_budget));
  }
  uint32_t expected_chunk_count = 1 + expected_continuations;

  /* Temporary buffer for continuation messages */
  size_t pkt_buf_size = session->packet_size;
  uint8_t *pkt_buf = (uint8_t *)malloc(pkt_buf_size);
  if (!pkt_buf)
    return NIPC_NP_ERR_ALLOC;

  for (uint32_t ci = 1; assembled < hdr_out->payload_len; ci++) {
    DWORD cn = 0;
    rerr = controlled_receive
               ? raw_recv_with_wait(session, pkt_buf, pkt_buf_size, &wait, &cn)
               : raw_recv_blocking(session, pkt_buf, pkt_buf_size, &cn);
    if (rerr != NIPC_NP_OK) {
      free(pkt_buf);
      return rerr;
    }

    if ((size_t)cn < NIPC_HEADER_LEN) {
      free(pkt_buf);
      return NIPC_NP_ERR_CHUNK;
    }

    nipc_chunk_header_t chk;
    perr = nipc_chunk_header_decode(pkt_buf, (size_t)cn, &chk);
    if (perr != NIPC_OK) {
      free(pkt_buf);
      return NIPC_NP_ERR_CHUNK;
    }

    /* Validate chunk header. A sender that uses smaller chunks than the
     * budget would need more chunks than the count it must declare. */
    if (chk.message_id != hdr_out->message_id || chk.chunk_index != ci ||
        ci >= expected_chunk_count ||
        chk.chunk_count != expected_chunk_count ||
        chk.total_message_len != (uint32_t)total_msg) {
      free(pkt_buf);
      return NIPC_NP_ERR_CHUNK;
    }

    size_t chunk_data = (size_t)cn - NIPC_HEADER_LEN;
    if (chunk_data != chk.chunk_payload_len) {
      free(pkt_buf);
      return NIPC_NP_ERR_CHUNK;
    }

    if (assembled + chunk_data > hdr_out->payload_len) {
      free(pkt_buf);
      return NIPC_NP_ERR_CHUNK;
    }

    memcpy(session->recv_buf + assembled, pkt_buf + NIPC_HEADER_LEN,
           chunk_data);
    assembled += chunk_data;
  }

  free(pkt_buf);

  *payload_out = session->recv_buf;
  *payload_len_out = hdr_out->payload_len;

  nipc_np_error_t berr =
      validate_batch(hdr_out, *payload_out, *payload_len_out);
  if (berr != NIPC_NP_OK)
    return berr;

  return NIPC_NP_OK;
}

nipc_np_error_t nipc_np_wait_readable(nipc_np_session_t *session,
                                      uint32_t timeout_ms, bool *readable_out) {
  if (!session || !readable_out || session->pipe == INVALID_HANDLE_VALUE)
    return NIPC_NP_ERR_BAD_PARAM;
  if (nipc_np_session_has_pending(session)) {
    *readable_out = true;
    return NIPC_NP_OK;
  }
  nipc_np_error_t err = wait_readable(session->pipe, timeout_ms, readable_out);
  if (err == NIPC_NP_ERR_DISCONNECTED || err == NIPC_NP_ERR_RECV)
    inflight_fail_all(session);
  return err;
}

#endif /* _WIN32 || __MSYS__ */
