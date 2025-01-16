// SPDX-License-Identifier: GPL-3.0-or-later

/* This is a standalone implementation of the systemd notify protocol with no external dependencies.
 * It was adapted from the MIT-0 licensed implementation found in the sd_notify(3) man page provided
 * with systemd v257.1.
 *
 * The protocol itself is defined both in the aforementioned man page, as well as at https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html
 * Said protocol is guaranteed stable per systemd's normal stability guarantees for external APIs.
 * Notably for our usage, the protocol also requires that unrecognized messages are ignored, which
 * means we can safely send non-mandatory messages only supported in the newest versions of the protocol
 * and trust systemd to handle them sanely.
 *
 * We use this instead of linking against libsystemd for sd_notify() support so that we can still use it with our static builds.
 */

#define _GNU_SOURCE 1

#include <errno.h>
#include <inttypes.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <time.h>
#include <unistd.h>

#define _cleanup_(f) __attribute__((cleanup(f)))

static void closep(int *fd) {
  if (!fd || *fd < 0)
    return;

  close(*fd);
  *fd = -1;
}

/* Send a notification to the service manager.
 * The notification message should be UTF-8 compatible text consisting of one item per line.
 */
static int notify(const char *message) {
  union sockaddr_union {
    struct sockaddr sa;
    struct sockaddr_un sun;
  } socket_addr = {
    .sun.sun_family = AF_UNIX,
  };
  size_t path_length, message_length;
  _cleanup_(closep) int fd = -1;
  const char *socket_path;

  /* Verify the argument first */
  if (!message)
    return -EINVAL;

  message_length = strlen(message);
  if (message_length == 0)
    return -EINVAL;

  /* If the variable is not set, the protocol is a noop */
  socket_path = getenv("NOTIFY_SOCKET");
  if (!socket_path)
    return 0; // Not set? Nothing to do

  /* Only AF_UNIX is supported, with path or abstract sockets */
  if (socket_path[0] != '/' && socket_path[0] != '@')
    return -EAFNOSUPPORT;

  path_length = strlen(socket_path);
  /* Ensure there is room for NUL byte */
  if (path_length >= sizeof(socket_addr.sun.sun_path))
    return -E2BIG;

  memcpy(socket_addr.sun.sun_path, socket_path, path_length);

  /* Support for abstract socket */
  if (socket_addr.sun.sun_path[0] == '@')
    socket_addr.sun.sun_path[0] = 0;

  fd = socket(AF_UNIX, SOCK_DGRAM|SOCK_CLOEXEC, 0);
  if (fd < 0)
    return -errno;

  if (connect(fd, &socket_addr.sa, offsetof(struct sockaddr_un, sun_path) + path_length) != 0)
    return -errno;

  ssize_t written = write(fd, message, message_length);
  if (written != (ssize_t) message_length)
    return written < 0 ? -errno : -EPROTO;

  return 1; // Notified!
}

/* Notify the service manager that Netdata has finished startup successfully.
 * This should be called only after we are sure we won’t exit due to
 * some issue with the configuration or environment.
 */
int notify_ready(void) {
  return notify("READY=1");
}

/* Notify the service manager that Netdata is reloading.
 * This should be called at the start of a configuration reload, and if called notify_read() _MUST_ be called when the configuration reload finishes.
 */
int notify_reloading(void) {
  /* A buffer with length sufficient to format the maximum UINT64 value. */
  size_t msg_length = sizeof("RELOADING=1\nMONOTONIC_USEC=18446744073709551615");
  char reload_message[msg_length];
  struct timespec ts;
  uint64_t now;

  /* Notify systemd that we are reloading, including a CLOCK_MONOTONIC timestamp in usec
   * so that the program is compatible with a Type=notify-reload service. */

  if (clock_gettime(CLOCK_MONOTONIC, &ts) < 0)
    return -errno;

  if (ts.tv_sec < 0 || ts.tv_nsec < 0 ||
      (uint64_t) ts.tv_sec > (UINT64_MAX - (ts.tv_nsec / 1000ULL)) / 1000000ULL)
    return -EINVAL;

  now = (uint64_t) ts.tv_sec * 1000000ULL + (uint64_t) ts.tv_nsec / 1000ULL;

  if (snprintf(reload_message, sizeof(reload_message), "RELOADING=1\nMONOTONIC_USEC=%" PRIu64, now) < 0)
    return -EINVAL;

  return notify(reload_message);
}

#define ND_EXTEND_TIMEOUT_USEC "EXTEND_TIMEOUT_USEC=18446744073709551615"

/* Request a service timeout extension from the service manager.
 *
 * The timeout should be a number in microseconds indicating the desired
 * service timeout extension
 */
int notify_extend_timeout(uint64_t timeout) {
  size_t msg_length = sizeof(ND_EXTEND_TIMEOUT_USEC);
  char message[msg_length];

  snprintf(message, msg_length, "EXTEND_TIMEOUT_USEC=%lu", timeout);

  return notify(message);
}

/* Notify the service manager that Netdata is stopping.
 * This should be called during the clean exit path.
 *
 * The timeout should be a number in microseconds indicating the desired
 * service timeout extension in the stop path (IOW, an upper bound on
 * how long we think it will take to stop).
 */
int notify_stopping(uint64_t timeout) {
  size_t msg_length = sizeof("STOPPING=1\n") + sizeof(ND_EXTEND_TIMEOUT_USEC);
  /* A buffer with length sufficient to format the maximum UINT64 value. */
  char stop_message[msg_length];

  snprintf(stop_message, msg_length, "STOPPING=1\nEXTEND_TIMEOUT_USEC=%lu", timeout);

  return notify(stop_message);
}

/* Send a status message to the service manager.
 * These are used to indicate what step is happening during startup or shutdown.
 */
int notify_status(const char *message) {
  size_t msg_length = strlen(message) + 9;
  // We have an empty message
  if (msg_length < 10) {
      return -EINVAL;
  }
  char status_message[msg_length];

  snprintf(status_message, msg_length, "STATUS=%s", message);

  return notify(status_message);
}
