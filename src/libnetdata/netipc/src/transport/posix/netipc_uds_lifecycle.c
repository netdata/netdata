#include "netipc_uds_internal.h"

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>

static pthread_mutex_t bind_umask_lock = PTHREAD_MUTEX_INITIALIZER;

static int bind_owner_only_socket(int fd, const struct sockaddr_un *addr)
{
    int rc = pthread_mutex_lock(&bind_umask_lock);
    if (rc != 0) {
        errno = rc;
        return -1;
    }

    /* Bind path sockets as 0600 without a post-bind permission window. */
    // Flawfinder: ignore
    mode_t old_mask = umask(S_IXUSR | S_IRWXG | S_IRWXO); // nosemgrep // NOSONAR
    int bind_rc = bind(fd, (const struct sockaddr *)addr, sizeof(*addr));
    int saved_errno = errno;
    // Flawfinder: ignore
    umask(old_mask); // nosemgrep // NOSONAR

    rc = pthread_mutex_unlock(&bind_umask_lock);
    if (bind_rc < 0) {
        errno = saved_errno;
        return -1;
    }
    if (rc != 0) {
        errno = rc;
        return -1;
    }
    return 0;
}

static int copy_cstr_checked(char *dst, size_t dst_size, const char *src)
{
    if (!dst || !src || dst_size == 0)
        return -1;

    size_t len = 0;
    while (len < dst_size && src[len] != '\0')
        len++;
    if (len == dst_size)
        return -1;

    memcpy(dst, src, len + 1);
    return 0;
}

static int fill_sockaddr_path(struct sockaddr_un *addr, const char *path)
{
    memset(addr, 0, sizeof(*addr));
    addr->sun_family = AF_UNIX;
    return copy_cstr_checked(addr->sun_path, sizeof(addr->sun_path), path);
}

static int validate_service_name(const char *name)
{
    if (!name || name[0] == '\0')
        return -1;

    if (name[0] == '.' && (name[1] == '\0' ||
                           (name[1] == '.' && name[2] == '\0')))
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

int nipc_uds_build_socket_name(char *dst, size_t dst_len,
                               const char *service_name)
{
    if (validate_service_name(service_name) < 0)
        return -2;

    int n = snprintf(dst, dst_len, "%s.sock", service_name);
    if (n < 0 || (size_t)n >= dst_len)
        return -1;
    return 0;
}

int nipc_uds_build_socket_path(char *dst, size_t dst_len,
                               const char *run_dir,
                               const char *service_name)
{
    char socket_name[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int name_rc = nipc_uds_build_socket_name(socket_name, sizeof(socket_name),
                                             service_name);
    if (name_rc < 0)
        return name_rc;

    int n = snprintf(dst, dst_len, "%s/%s", run_dir, socket_name);
    if (n < 0 || (size_t)n >= dst_len)
        return -1;
    return 0;
}

static int unlink_stale_socket_path(const char *run_dir,
                                    const char *socket_name)
{
    DIR *dir = opendir(run_dir);
    if (!dir)
        return -1;

    int dir_fd = dirfd(dir);
    if (dir_fd < 0) {
        closedir(dir);
        return -1;
    }

    /* Whatever sits at the endpoint name is not a live server (the connect
     * probe already failed): a dead server's socket or a foreign file.
     * Reclaim the name; directories are junk too. */
    int ret = -1;
    if (unlinkat(dir_fd, socket_name, 0) == 0 || errno == ENOENT)
        ret = 0;
    else if (errno == EISDIR && unlinkat(dir_fd, socket_name, AT_REMOVEDIR) == 0)
        ret = 0;

    closedir(dir);
    return ret;
}

int nipc_uds_check_and_recover_stale(const char *run_dir,
                                     const char *socket_name,
                                     const char *path)
{
    int probe = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (probe < 0) {
        /* Cannot probe liveness (fd exhaustion) — keep the endpoint and
         * report it in use rather than risk deleting a live socket. */
        return 1;
    }

    struct sockaddr_un addr;
    if (fill_sockaddr_path(&addr, path) != 0) {
        close(probe);
        return -1;
    }

    int ret;
    if (connect(probe, (struct sockaddr *)&addr, sizeof(addr)) == 0) {
        close(probe);
        ret = 1;
    } else {
        int saved_errno = errno;
        close(probe);
        if (saved_errno == ENOENT) {
            ret = -1;
        } else {
            ret = (unlink_stale_socket_path(run_dir, socket_name) == 0) ? 0 : 1;
        }
    }
    return ret;
}

nipc_uds_error_t nipc_uds_listen(const char *run_dir,
                                  const char *service_name,
                                  const nipc_uds_server_config_t *config,
                                  nipc_uds_listener_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc = nipc_uds_build_socket_path(path, sizeof(path), run_dir,
                                             service_name);
    if (path_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    char socket_name[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int name_rc = nipc_uds_build_socket_name(socket_name, sizeof(socket_name),
                                             service_name);
    if (name_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (name_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    int stale = nipc_uds_check_and_recover_stale(run_dir, socket_name, path);
    if (stale == 1)
        return NIPC_UDS_ERR_ADDR_IN_USE;

    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    if (fill_sockaddr_path(&addr, path) != 0) {
        close(fd);
        return NIPC_UDS_ERR_PATH_TOO_LONG;
    }

    if (bind_owner_only_socket(fd, &addr) < 0) {
        close(fd);
        return NIPC_UDS_ERR_SOCKET;
    }

    int backlog = config->backlog > 0 ? config->backlog : UDS_DEFAULT_BACKLOG;
    if (listen(fd, backlog) < 0) {
        close(fd);
        unlink(path);
        return NIPC_UDS_ERR_SOCKET;
    }

    out->fd = fd;
    out->config = *config;
    if (copy_cstr_checked(out->path, sizeof(out->path), path) != 0) {
        close(fd);
        unlink(path);
        memset(out, 0, sizeof(*out));
        out->fd = -1;
        return NIPC_UDS_ERR_PATH_TOO_LONG;
    }

    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_accept(nipc_uds_listener_t *listener,
                                  uint64_t session_id,
                                  nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    int client_fd = accept(listener->fd, NULL, NULL);
    if (client_fd < 0)
        return NIPC_UDS_ERR_ACCEPT;

    nipc_uds_error_t err = nipc_uds_server_handshake(
        client_fd, &listener->config, session_id, out);
    if (err != NIPC_UDS_OK) {
        close(client_fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_connect(const char *run_dir,
                                   const char *service_name,
                                   const nipc_uds_client_config_t *config,
                                   nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc = nipc_uds_build_socket_path(path, sizeof(path), run_dir,
                                             service_name);
    if (path_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    if (fill_sockaddr_path(&addr, path) != 0) {
        close(fd);
        return NIPC_UDS_ERR_PATH_TOO_LONG;
    }

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return NIPC_UDS_ERR_CONNECT;
    }

    nipc_uds_error_t err = nipc_uds_client_handshake(fd, config, out);
    if (err != NIPC_UDS_OK) {
        close(fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

void nipc_uds_close_session(nipc_uds_session_t *session)
{
    if (!session)
        return;

    if (session->fd >= 0) {
        close(session->fd);
        session->fd = -1;
    }

    free(session->recv_buf);
    session->recv_buf = NULL;
    session->recv_buf_size = 0;

    free(session->inflight_ids);
    session->inflight_ids = NULL;
    session->inflight_count = 0;
    session->inflight_capacity = 0;
}

void nipc_uds_close_listener(nipc_uds_listener_t *listener)
{
    if (!listener)
        return;

    if (listener->fd >= 0) {
        close(listener->fd);
        listener->fd = -1;
    }

    if (listener->path[0]) {
        unlink(listener->path);
        listener->path[0] = '\0';
    }
}
