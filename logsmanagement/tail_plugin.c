/** @file tail_plugin.c
 *  @brief Plugin to tail log files.
 *
 *  @author Dimitris Pantazis
 */

#include "tail_plugin.h"
#include "../daemon/common.h"
#include "../libnetdata/libnetdata.h"
#include <assert.h>
#include <inttypes.h>
#include <lz4.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include <uv.h>
#include "circular_buffer.h"
#include "logsmanagement_conf.h"
#include "helper.h"
#include "tail_plugin.h"

static uv_thread_t fs_events_reenable_thread_id;
extern uv_loop_t *main_loop; 

// Forward declarations
static void file_changed_cb(uv_fs_event_t *handle, const char *file_basename, int events, int status);
static void handle_UV_ENOENT_err(struct File_info *p_file_info);

/**
 * @brief This function will re-enable event listening for log files.
 *
 * @details This function runs on a separate thread and blocks using a condition
 * variable. It unblocks only when there is work to be done in order to restart
 * event listening for log file sources that were stopped previously for some
 * reason.
 * 
 * @param arg Unused.
 * */
static void fs_events_reenable_thread(void *arg) {
    UNUSED(arg);
    uint64_t fs_events_reenable_list_local = 0;
    int rc;
    while (1) {
        uv_mutex_lock(&p_file_infos_arr->fs_events_reenable_lock);
        while (p_file_infos_arr->fs_events_reenable_list == 0) {
            uv_cond_wait(&p_file_infos_arr->fs_events_reenable_cond,
                         &p_file_infos_arr->fs_events_reenable_lock);
        }
        fs_events_reenable_list_local = p_file_infos_arr->fs_events_reenable_list;
        p_file_infos_arr->fs_events_reenable_list = 0;
        uv_mutex_unlock(&p_file_infos_arr->fs_events_reenable_lock);

        // Give it some time for the file that wasn't found to be created
        uv_sleep(FS_EVENTS_REENABLE_INTERVAL);  

        debug(D_LOGS_MANAG, "fs_events_reenable_list pending: %" PRIu64, fs_events_reenable_list_local);
        for (int offset = 0; offset < p_file_infos_arr->count; offset++) {
            if (BIT_CHECK(fs_events_reenable_list_local, offset)) {
                debug(D_LOGS_MANAG, "Attempting to reenable fs_events for %s\n", p_file_infos_arr->data[offset]->filename);
                struct File_info *p_file_info = p_file_infos_arr->data[offset];
                debug(D_LOGS_MANAG, "Scheduling uv_fs_event for %s\n", p_file_info->filename);
                debug(D_LOGS_MANAG, "Current filesize in fs_events_reenable_thread: %" PRIu64, p_file_info->filesize);
                rc = uv_fs_event_start(p_file_info->fs_event_req, file_changed_cb, p_file_info->filename, 0);
                if (unlikely(rc)) {
                    debug(D_LOGS_MANAG, "uv_fs_event_start() for %s failed (%d): %s", p_file_info->filename, rc, uv_strerror(rc));
                    if (rc == UV_ENOENT) handle_UV_ENOENT_err(p_file_info);
                    else m_assert(!rc, "uv_fs_event_start() failed");
                }
            }
        }
    }
}

/**
 * @brief This function will handle libuv's "no such file or directory" error.
 *
 * @details When a file cannot be found, the function will stop the FS events 
 * handling registered on that file and inform the fs_events_reenable_thread 
 * that there is work to be done for that file to re-enable the event listening.
 *
 * @todo Once the dynamic configuration of files to be monitored has been 
 * implemented, this function should only try to restart the event listener only
 * if the file remains in the configuration.
 * @todo Current limit of 64 files max, due to size of fs_events_reenable_list.
 * */
static void handle_UV_ENOENT_err(struct File_info *const p_file_info) {
    // Stop fs events
    int rc = uv_fs_event_stop(p_file_info->fs_event_req);
    if (unlikely(rc)){
        error("uv_fs_event_stop() for %s failed:%s", p_file_info->filename, uv_strerror(rc));
    }
    m_assert(!rc, "uv_fs_event_stop() failed");

    p_file_info->force_file_changed_cb = 1;

    // Get offset of p_file_info in p_file_infos_arr that caused the UV_ENOENT error.
    int offset = 0;
    for (offset = 0; offset < p_file_infos_arr->count; offset++) {
        if (p_file_infos_arr->data[offset] == p_file_info) {
            debug(D_LOGS_MANAG, "handle_UV_ENOENT_err called for: %s", p_file_infos_arr->data[offset]->filename);
            break;
        }
    }

    p_file_info->filesize = 0;

    // Send signal to re-enable fs events
    uv_mutex_lock(&p_file_infos_arr->fs_events_reenable_lock);
    BIT_SET(p_file_infos_arr->fs_events_reenable_list, offset);
    uv_cond_signal(&p_file_infos_arr->fs_events_reenable_cond);
    uv_mutex_unlock(&p_file_infos_arr->fs_events_reenable_lock);
}

/**
 * @brief Closes a log file
 * @details Synchronously close a file using the libuv API. 
 * @param p_file_info #File_info struct containing the necessary data to close 
 * the file.
 */
static int file_close(struct File_info *const p_file_info) {
    int rc = 0;
    uv_fs_t close_req;
    rc = uv_fs_close(main_loop, &close_req, p_file_info->file_handle, NULL);
    if (unlikely(rc)) {
        error("error closing %s: %s", p_file_info->filename, uv_strerror(rc));
        m_assert(!rc, "uv_fs_close() failed");
    }
    uv_fs_req_cleanup(&close_req);
    return rc;
}

/**
 * @brief Opens a log file
 * @details Synchronously open a file (as read-only) using the libuv API. 
 * @param p_file_info #File_info struct containing the necessary data to open 
 * the file.
 */
static int file_open(struct File_info *const p_file_info) {
    int rc = 0;
    // TODO: More elegant solution is required - what if file becomes available later than startup?
    uv_fs_t open_req;
    rc = uv_fs_open(main_loop, &open_req, p_file_info->filename, O_RDONLY, 0, NULL);
    if (unlikely(rc < 0)) {
        error("file_open() error: %s (%d) %s", p_file_info->filename, rc, uv_strerror(rc));
        m_assert(rc == -2, "file_open() failed with rc != 2 (other than no such file or directory)");
    } else {
        // open_req->result of a uv_fs_t is the file descriptor in case of the uv_fs_open
        p_file_info->file_handle = open_req.result;  
    }
    uv_fs_req_cleanup(&open_req);
    return rc;
}

/**
 * @brief Timer callback function to re-enable file changed events. 
 * @details The function is responsible for re-enabling the FS event handle 
 * callback. Because there may be messages that were missed when the file events
 * were not being processed (i.e. for the duration of p_file_info->update_every), 
 * a "forced" call of #file_changed_cb() will take place (if and only if the 
 * #force_file_changed_cb variable is set). The value of #force_file_changed_cb 
 * is set or cleared in #enable_file_changed_events.
 * @param[in] handle Timer handle.
 */
static void enable_file_changed_events_timer_cb(uv_timer_t *handle) {
    int rc = 0;
    struct File_info *const p_file_info = handle->data;

    // debug(D_LOGS_MANAG, "Scheduling uv_fs_event for %s\n", p_file_info->filename);
    rc = uv_fs_event_start(p_file_info->fs_event_req, file_changed_cb, p_file_info->filename, 0);
    if (unlikely(rc)) {
        error("uv_fs_event_start() for %s failed (%d): %s\n", p_file_info->filename, rc, uv_strerror(rc));
        if (rc == UV_ENOENT) {
            handle_UV_ENOENT_err(p_file_info);
        } else
            m_assert(!rc, "uv_fs_event_start() failed");
    }

    if (p_file_info->force_file_changed_cb) {
        // debug(D_LOGS_MANAG, "Forcing uv_fs_event for %s\n", p_file_info->filename);
        file_changed_cb((uv_fs_event_t *)handle, p_file_info->file_basename, 0, 0);
    }
}

/**
 * @brief Starts a timer that will re-enable file_changed events when it expires
 * @details The purpose of this function is to re-enable file events but limit 
 * them to a maximum of 1 per p_file_info->update_every, to reduce CPU usage. 
 * @param p_file_info Struct containing the timer handle associated with the 
 * respective file info struct.
 * @param force_file_changed_cb Boolean variable. If set, a call to 
 * file_changed_cb() will be "force" as soon as the file event monitoring has 
 * been re-enabled. 
 * @return 0 on success
 */
static int enable_file_changed_events(struct File_info *p_file_info, uint8_t force_file_changed_cb) {
    int rc = 0;
    p_file_info->enable_file_changed_events_timer->data = p_file_info;
    p_file_info->force_file_changed_cb = force_file_changed_cb;
    // TODO: Change the timer implementation to start once, there is no reason to start-stop.
    rc = uv_timer_start(p_file_info->enable_file_changed_events_timer,
                        (uv_timer_cb)enable_file_changed_events_timer_cb, 
                        p_file_info->update_every * MSEC_PER_SEC, 0);
    if (unlikely(rc)) {
        error("uv_timer_start() error: (%d) %s\n", rc, uv_strerror(rc));
        m_assert(!rc, "uv_timer_start() error");
    }
    return rc;
}

/**
 * @brief Callback to digest buffer of read logs from log file.
 * @details Callback called in #check_if_filesize_changed_cb() after
 * logs I/O completed into buffer.
 */
static void read_file_cb(uv_fs_t *req) {
    struct File_info *p_file_info = req->data;
    Circ_buff_t *buff = p_file_info->circ_buff;

    if (likely(req->result > 0)) {
        buff->in->timestamp = get_unix_time_ms();
        m_assert(TEST_MS_TIMESTAMP_VALID(buff->in->timestamp), "buff->in->timestamp is invalid"); // Timestamp within valid range up to 2050
        buff->in->text_size = (size_t) req->result;

        /* Check if a half-line was read */
        while(buff->in->data[buff->in->text_size - 1] != '\n') { 
            buff->in->text_size--;
            if(unlikely(buff->in->text_size == 0)) goto free_access_lock;
        }

        /* Replace last '\n' with '\0' to null-terminate text */
        buff->in->data[buff->in->text_size - 1] = '\0'; 

        /* Store status and text_size in the buffer */
        buff->in->status = CIRC_BUFF_ITEM_STATUS_UNPROCESSED;

        /* Load max size of compressed buffer, as calculated previously */
        size_t text_compressed_buff_max_size = buff->in->text_compressed_size;

        /* Do compression */
        buff->in->text_compressed = buff->in->data + buff->in->text_size;
        buff->in->text_compressed_size = LZ4_compress_fast( buff->in->data, buff->in->text_compressed, 
                                                            buff->in->text_size, text_compressed_buff_max_size, 
                                                            p_file_info->compression_accel);
        m_assert(buff->in->text_compressed_size != 0, "Text_compressed_size should be != 0");

        // TODO: Validate compression option?

        /* Prepare log source read filesize for next collection 
         * important: needs to happen before circ_buff_insert() */
        p_file_info->filesize += buff->in->text_size;

        circ_buff_insert(buff);

        /* Instruct log parsing and metrics extraction */
        uv_mutex_lock(&p_file_info->notify_parser_thread_mut);
        p_file_info->log_batches_to_be_parsed++;
        uv_cond_signal(&p_file_info->notify_parser_thread_cond);
        uv_mutex_unlock(&p_file_info->notify_parser_thread_mut);

        goto free_access_lock;
    } 
    else if (unlikely(req->result < 0)) {
        error("Read error: %s for %s", uv_strerror(req->result), p_file_info->filename);
        m_assert(0, "Read error");
        goto free_access_lock;
    } 
    else if (unlikely(req->result == 0)) {
        error("Read error: %s for %s", uv_strerror(req->result), p_file_info->filename);
        m_assert(0, "Should never reach EOF");
        goto free_access_lock;
    }

free_access_lock:
    // debug(D_LOGS_MANAG, "Access_lock released for %s\n", p_file_info->file_basename);
    (void)file_close(p_file_info);
    (void)enable_file_changed_events(p_file_info, 1);
    uv_fs_req_cleanup(req);
}

/**
 * @brief Async callback function, called when a log file change event is fired.
 * @param[in] handle Callback handle to pass data to the function.
 * @param[in] file_basename Basename of the file that was changed.
 * @param[in] events ORed mask of uv_fs_event elements, unused.
 * @param[in] status Unused.
 */
static void file_changed_cb(uv_fs_event_t *handle, const char *file_basename, int events, int status) {
    UNUSED(file_basename);
    UNUSED(events);
    UNUSED(status);

    uv_fs_t stat_req;
    uv_stat_t *statbuf;

    int rc = 0;
    struct File_info *p_file_info = handle->data;

    // debug(D_LOGS_MANAG, "File changed! %s", file_basename ? file_basename : "");
    // debug(D_LOGS_MANAG, "%s %s", file_basename, 
    //         !p_file_info->force_file_changed_cb ? "forced file_changed_cb()" : "regular file_changed_cb()");

    rc = uv_fs_event_stop(p_file_info->fs_event_req);
    if (unlikely(rc)) {
        error("uv_fs_event_stop() error for %s: %s", p_file_info->filename, uv_strerror(rc));
        if (rc == UV_ENOENT) handle_UV_ENOENT_err(p_file_info);
        else m_assert(!rc, "uv_fs_event_stop() failed");
        goto return_error;
    }

    rc = uv_fs_stat(main_loop, &stat_req, p_file_info->filename, NULL);
    if (unlikely(rc)) {
        error("uv_fs_stat error: %s", uv_strerror(rc));
        if (rc == UV_ENOENT) handle_UV_ENOENT_err(p_file_info);
        else m_assert(!rc, "uv_fs_stat error");
        goto return_error;
    }
    statbuf = uv_fs_get_statbuf(&stat_req);    
    
    if(unlikely(p_file_info->rotated)){
        p_file_info->rotated = 0;
        p_file_info->filename[strlen(p_file_info->filename) - 2] = '\0';
        freez(p_file_info->file_basename);
        p_file_info->file_basename = get_basename(p_file_info->filename);
        // p_file_info->file_basename[strlen(p_file_info->file_basename - 2)] = '\0';
        p_file_info->filesize = 0;

        /* Need a new uv_fs_get_statbuf() call since filename changed */
        uv_fs_req_cleanup(&stat_req);
        rc = uv_fs_stat(main_loop, &stat_req, p_file_info->filename, NULL);
        if (unlikely(rc)) {
            error("uv_fs_stat error: %s\n", uv_strerror(rc));
            m_assert(!rc, "uv_fs_stat error");
            // TODO: Handle error in this case
        }
        statbuf = uv_fs_get_statbuf(&stat_req);

    } else if(unlikely(p_file_info->inode != statbuf->st_ino)) {
        char *new_filename = mallocz(strlen(p_file_info->filename)+ 3);
        new_filename[0] = '\0';
        strcat(new_filename, p_file_info->filename);
        strcat(new_filename, ".1");
        freez(p_file_info->filename);
        p_file_info->filename = new_filename;
        freez((void *) p_file_info->file_basename);
        p_file_info->file_basename = get_basename(p_file_info->filename);

        p_file_info->rotated = 1;
        p_file_info->inode = statbuf->st_ino;

        /* Need a new uv_fs_get_statbuf() call since filename changed */
        uv_fs_req_cleanup(&stat_req);
        rc = uv_fs_stat(main_loop, &stat_req, p_file_info->filename, NULL);
        if (unlikely(rc)) {
            error("uv_fs_stat error: %s", uv_strerror(rc));
            m_assert(!rc, "uv_fs_stat error");
            // TODO: Handle error in this case
        }
        statbuf = uv_fs_get_statbuf(&stat_req);
    }

    m_assert(p_file_info->filename, "p_file_info->filename is NULL");
    m_assert(p_file_info->file_basename, "p_file_info->file_basename is NULL");

    uint64_t new_filesize = statbuf->st_size;
    uint64_t old_filesize = p_file_info->filesize;
    uv_fs_req_cleanup(&stat_req);

    rc = file_open(p_file_info);
    if (unlikely(rc < 0)) {
        error("Error in file_open() (%d): %s", rc, uv_strerror(rc));
        if (rc == UV_ENOENT) handle_UV_ENOENT_err(p_file_info);
        else m_assert(0, "Error in file_open()");
        goto return_error;
    }

    /* CASE 1: Filesize has increased */
    if (likely((int64_t)new_filesize - (int64_t)old_filesize > 0)) {
        Circ_buff_t *buff = p_file_info->circ_buff;
        size_t filesize_diff = (size_t)(new_filesize - old_filesize);
        size_t available_text_space = circ_buff_prepare_write(buff, filesize_diff);

        if(unlikely(available_text_space == 0)){
            m_assert(available_text_space != 0, "available_text_space is 0");
            error("Circular buff for %s out of space! Will not collect anything in this iteration!", p_file_info->file_basename);
            goto return_error;
        }

        m_assert(available_text_space == filesize_diff, "available_text_space should be == filesize_diff");

        p_file_info->read_req.data = p_file_info;
        p_file_info->uvBuf = uv_buf_init( buff->in->data, filesize_diff);
        rc = uv_fs_read(main_loop, &p_file_info->read_req, p_file_info->file_handle,
                        &p_file_info->uvBuf, 1, old_filesize, read_file_cb);
        if (unlikely(rc)) {
            error("uv_fs_read() error for %s", p_file_info->file_basename);
            m_assert(!rc, "uv_fs_read() failed");
            goto return_error;
        }
        
        goto return_OK;
    }
    /* CASE 2: Filesize remains the same */
    else if (unlikely(new_filesize == old_filesize)) {
        debug(D_LOGS_MANAG, "%s changed but filesize remains the same", p_file_info->file_basename);
    }
    /* CASE 3: Filesize reduced */
    else {
        // TODO: Filesize reduced - error or log archived?? For now just assert
        infoerr("Filesize of %s reduced by %" PRId64 "B",p_file_info->file_basename, (int64_t)new_filesize - (int64_t)old_filesize);
        m_assert(0, "Filesize reduced!");
        goto return_error;
    }

return_error:
    fflush(stderr);
    (void)file_close(p_file_info);
    (void)enable_file_changed_events(p_file_info, 0);

return_OK:
    return;
}

/**
 * @brief Function to add a log file tailing input to the logs management engine.
 * 
 * @details This function will open a file log source, it will add a file change
 * event listener to it and it will add a callback to be called in case a change
 * event is detected. Any async callbacks registered this way will use the main
 * loop.
 * 
 * @param[in] p_file_info #File_info struct pointer of the struct where the new
 * input metadata will be stored in.
 * 
 * @return 0 on success or -1 in case of error.
 */
int tail_plugin_add_input(struct File_info *const p_file_info){
    int rc = 0;

    if ((rc = file_open(p_file_info)) < 0) {
        error("file_open() for %s failed during monitor_log_file_init(): (%d) %s", 
                    p_file_info->filename, rc, uv_strerror(rc));
        m_assert(rc == -2, "file_open() failed during monitor_log_file_init() \
                            with rc != 2 (other than no such file or directory)");
        return -1;
    }

    /* Store initial filesize in file_info - synchronous */
    uv_fs_t stat_req;
    stat_req.data = p_file_info;
    rc = uv_fs_stat(main_loop, &stat_req, p_file_info->filename, NULL);
    if (unlikely(rc)) {
        error("uv_fs_stat() error for %s: (%d) %s", p_file_info->filename, rc, uv_strerror(rc));
        m_assert(!rc, "uv_fs_stat() failed during monitor_log_file_init()");
        uv_fs_req_cleanup(&stat_req);
        return -1;
    }

    /* uv_fs_stat() request succeeded; get filesize */
    uv_stat_t *statbuf = uv_fs_get_statbuf(&stat_req);
    debug(D_LOGS_MANAG, "Initial size of %s: %lldKB", p_file_info->filename, (long long)statbuf->st_size / 1000);
    p_file_info->filesize = statbuf->st_size;
    p_file_info->inode = statbuf->st_ino;
    uv_fs_req_cleanup(&stat_req);

    /* Initialise events listener */
    debug(D_LOGS_MANAG, "Adding changes listener for %s", p_file_info->file_basename);
    uv_fs_event_t *fs_event_req = mallocz(sizeof(uv_fs_event_t));
    fs_event_req->data = p_file_info;
    p_file_info->fs_event_req = fs_event_req;

    rc = uv_fs_event_init(main_loop, fs_event_req);
    if (unlikely(rc)) fatal("uv_fs_event_init() failed for %s: (%d) %s", p_file_info->filename, rc, uv_strerror(rc));

    p_file_info->enable_file_changed_events_timer = mallocz(sizeof(uv_timer_t));
    rc = uv_timer_init(main_loop, p_file_info->enable_file_changed_events_timer);
    if (unlikely(rc)) fatal("uv_timer_init() failed for %s: (%d) %s", p_file_info->filename, rc, uv_strerror(rc));

    uv_fs_event_start(p_file_info->fs_event_req, file_changed_cb, p_file_info->filename, 0);
    if (unlikely(rc)) fatal("uv_fs_event_start() failed for %s: (%d) %s", p_file_info->filename, rc, uv_strerror(rc));

    return 0;
}

void tail_plugin_init(struct File_infos_arr *const p_file_infos_arr){
    if(unlikely(0 != uv_mutex_init(&p_file_infos_arr->fs_events_reenable_lock))) fatal("uv_mutex_init() failed");
    if(unlikely(0 != uv_cond_init(&p_file_infos_arr->fs_events_reenable_cond))) fatal("uv_cond_init() failed");
    uv_thread_create(&fs_events_reenable_thread_id, fs_events_reenable_thread, NULL);
}
