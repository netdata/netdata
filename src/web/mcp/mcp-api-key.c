// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-api-key.h"
#include "claim/claim.h"
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY

static char mcp_dev_preview_api_key[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1] = "";

static bool mcp_api_key_generate_and_save(void) {
    nd_uuid_t uuid;
    uuid_generate_random(uuid);
    
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(uuid, uuid_str);
    
    // Create directory if it doesn't exist
    char *dir = "/var/lib/netdata";
    if (mkdir(dir, 0755) == -1 && errno != EEXIST) {
        netdata_log_error("MCP: Failed to create directory %s: %s", dir, strerror(errno));
        return false;
    }
    
    // Open file with O_CREAT | O_EXCL to ensure we don't overwrite
    int fd = open(MCP_DEV_PREVIEW_API_KEY_PATH, O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd == -1) {
        netdata_log_error("MCP: Failed to create API key file %s: %s", 
                         MCP_DEV_PREVIEW_API_KEY_PATH, strerror(errno));
        return false;
    }
    
    // Write the UUID
    ssize_t written = write(fd, uuid_str, strlen(uuid_str));
    if (written != (ssize_t)strlen(uuid_str)) {
        netdata_log_error("MCP: Failed to write API key to file: %s", strerror(errno));
        close(fd);
        unlink(MCP_DEV_PREVIEW_API_KEY_PATH);
        return false;
    }
    
    close(fd);
    
    // Ensure file permissions are correct (only owner can read/write)
    if (chmod(MCP_DEV_PREVIEW_API_KEY_PATH, 0600) == -1) {
        netdata_log_error("MCP: Failed to set permissions on API key file: %s", strerror(errno));
        unlink(MCP_DEV_PREVIEW_API_KEY_PATH);
        return false;
    }
    
    strncpy(mcp_dev_preview_api_key, uuid_str, MCP_DEV_PREVIEW_API_KEY_LENGTH);
    mcp_dev_preview_api_key[MCP_DEV_PREVIEW_API_KEY_LENGTH] = '\0';
    
    netdata_log_info("MCP: Generated new developer preview API key");
    return true;
}

static bool mcp_api_key_load(void) {
    int fd = open(MCP_DEV_PREVIEW_API_KEY_PATH, O_RDONLY);
    if (fd == -1) {
        if (errno == ENOENT) {
            // File doesn't exist, this is expected on first run
            return false;
        }
        netdata_log_error("MCP: Failed to open API key file %s: %s", 
                         MCP_DEV_PREVIEW_API_KEY_PATH, strerror(errno));
        return false;
    }
    
    char buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1];
    ssize_t bytes_read = read(fd, buffer, MCP_DEV_PREVIEW_API_KEY_LENGTH);
    close(fd);
    
    if (bytes_read != MCP_DEV_PREVIEW_API_KEY_LENGTH) {
        netdata_log_error("MCP: Invalid API key file size: expected %d bytes, got %zd", 
                         MCP_DEV_PREVIEW_API_KEY_LENGTH, bytes_read);
        return false;
    }
    
    buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH] = '\0';
    
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
    
    netdata_log_info("MCP: Developer preview API key initialized. Location: %s", MCP_DEV_PREVIEW_API_KEY_PATH);
}

bool mcp_api_key_verify(const char *api_key) {
    if (!api_key || !*api_key) {
        netdata_log_error("MCP: No API key provided");
        return false;
    }
    
    // Check if agent is claimed
    if (!is_agent_claimed()) {
        netdata_log_error("MCP: API key authentication rejected - agent is not claimed to Netdata Cloud");
        return false;
    }
    
    // Check if we have a loaded API key
    if (!mcp_dev_preview_api_key[0]) {
        netdata_log_error("MCP: No API key loaded");
        return false;
    }
    
    // Compare the keys
    bool valid = (strcmp(api_key, mcp_dev_preview_api_key) == 0);
    
    if (!valid) {
        netdata_log_error("MCP: Invalid API key provided");
    }
    
    return valid;
}

const char *mcp_api_key_get(void) {
    return mcp_dev_preview_api_key;
}

#endif // NETDATA_MCP_DEV_PREVIEW_API_KEY