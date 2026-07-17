// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp_auth.h"
#include "claim/claim.h"
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>
#include <limits.h>

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY

static char mcp_dev_preview_api_key[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1] = "";

static bool mcp_api_key_generate_and_save(void) {
    nd_uuid_t uuid;
    uuid_generate_random(uuid);

    // Unparse directly to the destination buffer
    uuid_unparse_lower(uuid, mcp_dev_preview_api_key);

    // Construct full path
    char path[PATH_MAX];
    snprintf(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, MCP_DEV_PREVIEW_API_KEY_FILENAME);

    // Write the UUID with newline
    char buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 2]; // +1 for newline, +1 for null
    snprintf(buffer, sizeof(buffer), "%s\n", mcp_dev_preview_api_key);
    const ssize_t expected = (ssize_t)(MCP_DEV_PREVIEW_API_KEY_LENGTH + 1); // +1 for newline

    struct stat st;
    int fd;

#ifdef O_NOFOLLOW
    // Open without O_TRUNC: truncation happens via ftruncate() only after fstat()
    // confirms a regular file. O_NONBLOCK avoids blocking on a FIFO/device and
    // O_NOFOLLOW refuses to open a symlink planted at the key path. fstat() on the
    // already-opened fd rejects any non-regular file without a TOCTOU window.
    fd = open(path, O_WRONLY | O_CREAT | O_CLOEXEC | O_NONBLOCK | O_NOFOLLOW, 0600);
    if (fd == -1) {
        netdata_log_error("MCP: Failed to create API key file %s: %s", path, strerror(errno));
        return false;
    }

    if (fstat(fd, &st) != 0 || !S_ISREG(st.st_mode)) {
        netdata_log_error("MCP: API key file '%s' is not a regular file.", path);
        close(fd);
        return false;
    }

    if (ftruncate(fd, 0) != 0) {
        netdata_log_error("MCP: Cannot truncate API key file '%s': %s", path, strerror(errno));
        close(fd);
        return false;
    }

    ssize_t written = write(fd, buffer, (size_t)expected);
    if (written != expected) {
        netdata_log_error("MCP: Failed to write API key to file: %s", strerror(errno));
        close(fd);
        return false;
    }

    if (close(fd) != 0) {
        netdata_log_error("MCP: Failed to close API key file '%s': %s", path, strerror(errno));
        return false;
    }
#else
    // Without O_NOFOLLOW: write to a uniquely named temp file then rename atomically.
    // mkstemp() yields an unpredictable name and rename() replaces the destination entry
    // without following a symlink planted there, so the key file cannot be redirected.
    char tmp_filename[PATH_MAX];
    snprintfz(tmp_filename, sizeof(tmp_filename), "%s.tmp.XXXXXX", path);

    fd = mkstemp(tmp_filename);
    if (fd == -1) {
        netdata_log_error("MCP: Failed to create temporary API key file '%s': %s", tmp_filename, strerror(errno));
        return false;
    }

    ssize_t written = write(fd, buffer, (size_t)expected);
    if (written != expected) {
        netdata_log_error("MCP: Failed to write API key to file: %s", strerror(errno));
        close(fd);
        unlink(tmp_filename);
        return false;
    }

    if (close(fd) != 0) {
        netdata_log_error("MCP: Failed to close API key file '%s': %s", tmp_filename, strerror(errno));
        unlink(tmp_filename);
        return false;
    }

    if (rename(tmp_filename, path) != 0) {
        netdata_log_error("MCP: Cannot rename temporary API key file '%s' to '%s': %s",
                         tmp_filename, path, strerror(errno));
        unlink(tmp_filename);
        return false;
    }
#endif

    // Ensure file permissions are correct (only owner can read/write)
    if (chmod(path, 0600) == -1) {
        netdata_log_error("MCP: Failed to set permissions on API key file: %s", strerror(errno));
        return false;
    }

    netdata_log_info("MCP: Generated new developer preview API key");
    return true;
}

static bool mcp_api_key_load(void) {
    // Construct full path
    char path[PATH_MAX];
    snprintf(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, MCP_DEV_PREVIEW_API_KEY_FILENAME);

#ifdef O_NOFOLLOW
    // O_NOFOLLOW refuses to open a symlink planted at the key path; O_NONBLOCK avoids
    // blocking on a FIFO/device. fstat() on the already-opened fd rejects any non-regular
    // file without a TOCTOU window (no lstat() before open()).
    int fd = open(path, O_RDONLY | O_CLOEXEC | O_NONBLOCK | O_NOFOLLOW);
#else
    int fd = open(path, O_RDONLY | O_CLOEXEC | O_NONBLOCK);
#endif

    if (fd == -1) {
        if (errno == ENOENT) {
            // File doesn't exist, this is expected on first run
            return false;
        }
        netdata_log_error("MCP: Failed to open API key file %s: %s", path, strerror(errno));
        return false;
    }

    struct stat st;
    if (fstat(fd, &st) != 0 || !S_ISREG(st.st_mode)) {
        netdata_log_error("MCP: API key file '%s' is not a regular file, regenerating.", path);
        close(fd);
        return false;
    }

    char buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 2]; // +1 for potential newline, +1 for null
    ssize_t bytes_read = read(fd, buffer, sizeof(buffer) - 1);
    close(fd);

    if (bytes_read < MCP_DEV_PREVIEW_API_KEY_LENGTH || bytes_read > MCP_DEV_PREVIEW_API_KEY_LENGTH + 1) {
        netdata_log_error("MCP: Invalid API key file size: expected %d or %d bytes, got %zd",
                         MCP_DEV_PREVIEW_API_KEY_LENGTH, MCP_DEV_PREVIEW_API_KEY_LENGTH + 1, bytes_read);
        return false;
    }

    buffer[bytes_read] = '\0';

    // Strip trailing newline if present
    if (bytes_read > 0 && buffer[bytes_read - 1] == '\n') {
        buffer[bytes_read - 1] = '\0';
    }

    // Basic validation - should be a valid UUID format
    nd_uuid_t uuid;
    if (uuid_parse(buffer, uuid) != 0) {
        netdata_log_error("MCP: Invalid UUID format in API key file");
        return false;
    }

    strncpy(mcp_dev_preview_api_key, buffer, MCP_DEV_PREVIEW_API_KEY_LENGTH);
    mcp_dev_preview_api_key[MCP_DEV_PREVIEW_API_KEY_LENGTH] = '\0';

    netdata_log_info("MCP: Loaded developer preview API key");
    return true;
}

void mcp_api_key_initialize(void) {
    // Try to load existing key first
    if (!mcp_api_key_load()) {
        // If loading fails, generate a new one
        if (!mcp_api_key_generate_and_save()) {
            netdata_log_error("MCP: Failed to initialize API key system");
            return;
        }
    }

    char path[PATH_MAX];
    snprintf(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, MCP_DEV_PREVIEW_API_KEY_FILENAME);
#if defined(OS_WINDOWS)
    char display_path[PATH_MAX];
    netdata_log_info("MCP: Developer preview API key initialized. Location: %s",
                     os_translate_path(display_path, path, sizeof(display_path)));
#else
    netdata_log_info("MCP: Developer preview API key initialized. Location: %s", path);
#endif
}

bool mcp_api_key_verify(const char *api_key, bool silent) {
    if (!api_key || !*api_key) {
        if (!silent)
            netdata_log_error("MCP: No API key provided");
        return false;
    }

    // Check if agent is claimed
    if (!is_agent_claimed()) {
        if (!silent)
            netdata_log_error("MCP: API key authentication rejected - agent is not claimed to Netdata Cloud");
        return false;
    }

    // Check if we have a loaded API key
    if (!mcp_dev_preview_api_key[0]) {
        if (!silent)
            netdata_log_error("MCP: No API key loaded");
        return false;
    }

    // Compare the keys
    bool valid = (strcmp(api_key, mcp_dev_preview_api_key) == 0);

    if (!valid && !silent) {
        netdata_log_error("MCP: Invalid API key provided");
    }

    return valid;
}

const char *mcp_api_key_get(void) {
    return mcp_dev_preview_api_key;
}

#endif // NETDATA_MCP_DEV_PREVIEW_API_KEY
