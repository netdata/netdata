#include "otel_ingest.hpp"

#include "daemon/common.h"

enum class InitStatus : unsigned int {
    Uninitialized = 0,
    HaveLoop = 1 << 0,
    HaveAsync = 1 << 1,
    HaveCompletion = 1 << 2,
    HaveMetricsFifo = 1 << 3,
    HaveLogsFifo = 1 << 4,
    HaveTracesFifo = 1 << 5,
    HaveSpawnedCollector = 1 << 6,
    HaveRunLoop = 1 << 7,
};

inline InitStatus operator|(InitStatus lhs, InitStatus rhs)
{
    return static_cast<InitStatus>(
        static_cast<std::underlying_type<InitStatus>::type>(lhs) |
        static_cast<std::underlying_type<InitStatus>::type>(rhs));
}

inline InitStatus operator&(InitStatus lhs, InitStatus rhs)
{
    return static_cast<InitStatus>(
        static_cast<std::underlying_type<InitStatus>::type>(lhs) &
        static_cast<std::underlying_type<InitStatus>::type>(rhs));
}

inline InitStatus &operator|=(InitStatus &lhs, InitStatus rhs)
{
    lhs = lhs | rhs;
    return lhs;
}

typedef enum {
    OTEL_FIFO_KIND_METRICS,
    OTEL_FIFO_KIND_LOGS,
    OTEL_FIFO_KIND_TRACES,
} otel_fifo_kind_t;

static const char *otel_fifo_kind_to_string(otel_fifo_kind_t otel_fifo_kind)
{
    switch (otel_fifo_kind) {
        case OTEL_FIFO_KIND_METRICS:
            return "metrics";
        case OTEL_FIFO_KIND_LOGS:
            return "logs";
        case OTEL_FIFO_KIND_TRACES:
            return "traces";
    }

    fatal("Unknown fifo kind: %d", otel_fifo_kind);
}

typedef struct {
    otel_fifo_kind_t kind;
    const char *path;
    int fd;
    uv_pipe_t *pipe;
} otel_fifo_t;

typedef struct otel_state {
    InitStatus init_status;

    uv_loop_t *loop;
    uv_process_t otel_process;
    struct completion otel_process_completion;

    otel_fifo_t metrics_fifo;
    otel_fifo_t logs_fifo;
    otel_fifo_t traces_fifo;

    uv_async_t async;
    struct completion shutdown_completion;

    unsigned loop_counter;

    bool haveLoop() const
    {
        return haveInitStatus(InitStatus::HaveLoop);
    }

    bool haveMetricsFifo() const
    {
        return haveInitStatus(InitStatus::HaveMetricsFifo);
    }

    bool haveLogsFifo() const
    {
        return haveInitStatus(InitStatus::HaveLogsFifo);
    }

    bool haveTracesFifo() const
    {
        return haveInitStatus(InitStatus::HaveTracesFifo);
    }

    bool haveAllFifos() const
    {
        return haveMetricsFifo() && haveLogsFifo() && haveTracesFifo();
    }

    bool haveSpawnedCollector() const
    {
        return haveInitStatus(InitStatus::HaveSpawnedCollector);
    }

    bool haveRunLoop() const
    {
        return haveInitStatus(InitStatus::HaveRunLoop);
    }

    bool haveAsync() const
    {
        return haveInitStatus(InitStatus::HaveAsync);
    }

    void dump() const
    {
        netdata_log_error(
            "[GVD] loop: %d, metrics: %d, logs: %d, traces: %d, spawned: %d, run: %d, async: %d (raw=%u)",
            haveLoop(),
            haveMetricsFifo(),
            haveLogsFifo(),
            haveTracesFifo(),
            haveSpawnedCollector(),
            haveRunLoop(),
            haveAsync(),
            static_cast<unsigned int>(init_status));
    }

private:
    bool haveInitStatus(const InitStatus Flag) const
    {
        return (init_status & Flag) != InitStatus::Uninitialized;
    }
} otel_state_t;

static otel_state_t otel_state;

static void alloc_buffer(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf)
{
    UNUSED(handle);

    suggested_size = 16 * 1024 * 1024;

    char *ptr = static_cast<char *>(callocz(suggested_size, sizeof(char)));
    if (!ptr)
        fatal("[OTEL] Could not allocate buffer for libuv");

    *buf = uv_buf_init(ptr, suggested_size);
}

static void on_read(uv_stream_t *stream, ssize_t nread, const uv_buf_t *buf)
{
    if (nread > 0) {
        otel::Otel *OT = reinterpret_cast<otel::Otel *>(stream->data);
        const uv_buf_t data = {.base = buf->base, .len = (size_t)nread};

        OT->processMessages(data);
    } else if (nread < 0) {
        if (nread == UV_EOF) {
            netdata_log_error("[GVD] Reached EOF...");
        } else {
            netdata_log_error("[GVD] Read error: %s", uv_strerror(nread));
        }
    }

    if (buf->base)
        free(buf->base);
}

static otel_fifo_t create_fifo(otel_fifo_kind_t otel_fifo_kind, void *data)
{
    otel_fifo_t otel_fifo = {
        .kind = otel_fifo_kind,
        .path = nullptr,
        .fd = -1,
        .pipe = nullptr,
    };

    const char *fifo_kind = otel_fifo_kind_to_string(otel_fifo_kind);

    char key[128 + 1];
    snprintfz(key, 128, "fifo path for %s", fifo_kind);

    char value[FILENAME_MAX + 1];
    snprintfz(value, FILENAME_MAX, "%s/otel-%s.fifo", netdata_configured_cache_dir, fifo_kind);

    otel_fifo.path = config_get(CONFIG_SECTION_OTEL, key, value);

    // remove any leftover files
    unlink(otel_fifo.path);

    // create fifo
    errno = 0;
    if (mkfifo(otel_fifo.path, 0664) != 0) {
        netdata_log_error(
            "Could not create %s FIFO at %s: %s (errno=%d)", fifo_kind, otel_fifo.path, strerror(errno), errno);
        otel_fifo.path = nullptr;
        return otel_fifo;
    }

    // open for reading
    otel_fifo.fd = open(otel_fifo.path, O_RDONLY | O_NONBLOCK);
    if (otel_fifo.fd == -1) {
        netdata_log_error(
            "Could not open %s FIFO at %s: %s (errno=%d)", fifo_kind, otel_fifo.path, strerror(errno), errno);

        unlink(otel_fifo.path);
        otel_fifo.path = NULL;
        return otel_fifo;
    }

    /*
     * create a uv_pipe out of the FIFO fd
    */

    otel_fifo.pipe = reinterpret_cast<uv_pipe_t *>(callocz(1, sizeof(uv_pipe_t)));
    int err = uv_pipe_init(otel_state.loop, otel_fifo.pipe, 0);
    if (err) {
        netdata_log_error("uv_pipe_init(): %s", uv_strerror(err));
        goto LBL_PIPE_ERROR;
    }

    err = uv_pipe_open(otel_fifo.pipe, otel_fifo.fd);
    if (err) {
        netdata_log_error("uv_pipe_open(): %s", uv_strerror(err));
        goto LBL_PIPE_ERROR;
    }

    otel_fifo.pipe->data = data;

    err = uv_read_start((uv_stream_t *)otel_fifo.pipe, alloc_buffer, on_read);
    if (err) {
        netdata_log_error("uv_read_start(): %s", uv_strerror(err));
        goto LBL_PIPE_ERROR;
    }

    switch (otel_fifo_kind) {
        case OTEL_FIFO_KIND_METRICS:
            otel_state.init_status |= InitStatus::HaveMetricsFifo;
            break;
        case OTEL_FIFO_KIND_LOGS:
            otel_state.init_status |= InitStatus::HaveLogsFifo;
            break;
        case OTEL_FIFO_KIND_TRACES:
            otel_state.init_status |= InitStatus::HaveTracesFifo;
            break;
    }

    return otel_fifo;

LBL_PIPE_ERROR:
    freez(otel_fifo.pipe);
    close(otel_fifo.fd);
    unlink(otel_fifo.path);

    return otel_fifo_t{
        .kind = otel_fifo_kind,
        .path = nullptr,
        .fd = -1,
        .pipe = nullptr,
    };
}

static void destroy_fifo(otel_fifo_t *otel_fifo)
{
    freez(otel_fifo->pipe);
    close(otel_fifo->fd);
    unlink(otel_fifo->path);

    memset(otel_fifo, 0, sizeof(otel_fifo_t));
}

static void spawn_otel_collector()
{
    uv_process_options_t options;
    memset(&options, 0, sizeof(uv_process_options_t));

    options.file = config_get("otel", "otel collector binary", "/usr/local/bin/otelcontribcol");

    char **args = new char *[4]{nullptr, nullptr, nullptr, nullptr};
    args[0] = strdupz(options.file);
    args[1] = strdupz("--config");
    {
        char path[FILENAME_MAX + 1];
        snprintfz(path, FILENAME_MAX, "%s/otel-config.yaml", netdata_configured_user_config_dir);

        const char *cfg_path = config_get("otel", "otel collector configuration file", path);

        args[2] = strdupz(cfg_path);
    }
    args[3] = nullptr;

    options.args = args;
    options.exit_cb = [](uv_process_t *req, int64_t exit_status, int term_signal) {
        netdata_log_error("GVD OTEL collector exit_status: %lu, term_signal: %d", exit_status, term_signal);
        char **arg = (char **)req->data;
        while (*arg)
            freez(*arg++);

        free(req->data);

        completion_mark_complete(&otel_state.otel_process_completion);
    };

    otel_state.otel_process.data = (void *)args;

    {
        // Set up stdio containers
        uv_stdio_container_t stdio[3];

        // stdin
        stdio[0].flags = UV_IGNORE;

        // stdout
        stdio[1].flags = UV_IGNORE;

        // stderr
        stdio[2].flags = UV_IGNORE;

        int fd_stderr = netdata_logger_fd(NDLS_COLLECTORS);
        if (fd_stderr != -1) {
            stdio[2].flags = UV_INHERIT_FD;
            stdio[2].data.fd = fd_stderr;
        }

        options.stdio_count = 3;
        options.stdio = stdio;
    }

    int err = uv_spawn(otel_state.loop, &otel_state.otel_process, &options);
    if (err) {
        netdata_log_error("GVD: failed to spawn otel collector....");
        char **arg = (char **)args;
        while (*arg)
            freez(*arg++);
        delete[] args;
    } else {
        netdata_log_error("GVD: spawned otel collector....");
        // For good measure...
        signals_restore_SIGCHLD();
        otel_state.init_status |= InitStatus::HaveSpawnedCollector;
    }
}

static void shutdown_libuv_handles(uv_async_t *handle)
{
    UNUSED(handle);

    // FIXME: the process shutdowns but the exit callback does not run
    // This times out the completion and we end up killing the process.
    // However the process will not get reaped by the agent.
    if (otel_state.haveRunLoop()) {
        uv_process_kill(&otel_state.otel_process, SIGTERM);

        bool ok = completion_timedwait_for(&otel_state.otel_process_completion, 3);
        if (!ok)
            uv_process_kill(&otel_state.otel_process, SIGKILL);
    }

    if (otel_state.haveMetricsFifo())
        uv_close((uv_handle_t *)otel_state.metrics_fifo.pipe, NULL);
    if (otel_state.haveLogsFifo())
        uv_close((uv_handle_t *)otel_state.logs_fifo.pipe, NULL);
    if (otel_state.haveTracesFifo())
        uv_close((uv_handle_t *)otel_state.traces_fifo.pipe, NULL);

    if (otel_state.haveAsync())
        uv_close((uv_handle_t *)&otel_state.async, NULL);

    if (otel_state.haveSpawnedCollector())
        uv_close((uv_handle_t *)&otel_state.otel_process, NULL);

    if (otel_state.haveRunLoop())
        uv_stop(otel_state.loop);
}

extern "C" void otel_init(void)
{
    memset(&otel_state, 0, sizeof(otel_state_t));

    otel_state.loop = reinterpret_cast<uv_loop_t *>(callocz(1, sizeof(uv_loop_t)));
    int err = uv_loop_init(otel_state.loop);
    if (err) {
        freez(otel_state.loop);
        netdata_log_error("GVD: Failed to initialize libuv loop: %s", uv_strerror(err));
        return;
    }
    otel_state.init_status |= InitStatus::HaveLoop;

    err = uv_async_init(otel_state.loop, &otel_state.async, shutdown_libuv_handles);
    if (err) {
        netdata_log_error("GVD: Failed to initialize async handle: %s", uv_strerror(err));
        return;
    }
    otel_state.init_status |= InitStatus::HaveAsync;

    completion_init(&otel_state.shutdown_completion);
    completion_init(&otel_state.otel_process_completion);
    otel_state.init_status |= InitStatus::HaveCompletion;

    {
        char path[FILENAME_MAX + 1];
        snprintfz(path, FILENAME_MAX, "%s/otel-receivers-config.yaml", netdata_configured_user_config_dir);

        const char *cfg_path = config_get("otel", "otel receivers configuration file", path);
        auto OT = otel::Otel::get(cfg_path);
        if (!OT.ok()) {
            std::stringstream SS;
            SS << OT.status();

            // TODO: non-fatal + cleanup
            fatal("%s", SS.str().c_str());
        }

        otel_state.metrics_fifo = create_fifo(OTEL_FIFO_KIND_METRICS, *OT);
        otel_state.logs_fifo = create_fifo(OTEL_FIFO_KIND_LOGS, nullptr);
        otel_state.traces_fifo = create_fifo(OTEL_FIFO_KIND_TRACES, nullptr);

        if (!otel_state.haveAllFifos()) {
            netdata_log_error("GVD: Could not create required FIFOs to collect OTEL data");
            return;
        }
    }

    spawn_otel_collector();
}

extern "C" void otel_shutdown(void)
{
    if (otel_state.haveRunLoop()) {
        uv_async_send(&otel_state.async);
        completion_wait_for(&otel_state.shutdown_completion);
        completion_destroy(&otel_state.shutdown_completion);
    }

    if (otel_state.haveMetricsFifo())
        destroy_fifo(&otel_state.metrics_fifo);
    if (otel_state.haveLogsFifo())
        destroy_fifo(&otel_state.logs_fifo);
    if (otel_state.haveTracesFifo())
        destroy_fifo(&otel_state.traces_fifo);

    if (otel_state.haveLoop())
        freez(otel_state.loop);
}

static void otel_main_cleanup(void *data)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // Nothing to do here everything's cleaned up during otel_shutdown()

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

extern "C" void *otel_main(void *ptr)
{
    netdata_thread_cleanup_push(otel_main_cleanup, ptr);

    if (otel_state.haveSpawnedCollector()) {
        otel_state.init_status |= InitStatus::HaveRunLoop;
        uv_run(otel_state.loop, UV_RUN_DEFAULT);
        completion_mark_complete(&otel_state.shutdown_completion);

        int ret = uv_loop_close(otel_state.loop);
        if (ret == UV_EBUSY)
            fatal("GVD P[OTEL] libuv loop closed with EBUSY");
    }

    netdata_thread_cleanup_pop(1);
    return nullptr;
}
