# SPDX-License-Identifier: GPL-3.0-or-later

COMMON_CFLAGS = -I ../../ -DTARGET_OS=1  -Wall -Wextra
PROFILE_CFLAGS = -O1 -ggdb $(COMMON_CFLAGS)
PERFORMANCE_CFLAGS = -O2 $(COMMON_CFLAGS)

CFLAGS = $(PERFORMANCE_CFLAGS)

LIBNETDATA_OBJS = \
    ../../libnetdata/popen/popen.o \
    ../../libnetdata/storage_number/storage_number.o \
    ../../libnetdata/avl/avl.o \
    ../../libnetdata/socket/socket.o \
    ../../libnetdata/os.o \
    ../../libnetdata/clocks/clocks.o \
    ../../libnetdata/procfile/procfile.o \
    ../../libnetdata/statistical/statistical.o \
    ../../libnetdata/eval/eval.o \
    ../../libnetdata/threads/threads.o \
    ../../libnetdata/dictionary/dictionary.o \
    ../../libnetdata/simple_pattern/simple_pattern.o \
    ../../libnetdata/url/url.o \
    ../../libnetdata/config/appconfig.o \
    ../../libnetdata/libnetdata.o \
    ../../libnetdata/buffer/buffer.o \
    ../../libnetdata/adaptive_resortable_list/adaptive_resortable_list.o \
    ../../libnetdata/locks/locks.o \
    ../../libnetdata/log/log.o \
    ../../libnetdata/socket/security.o \
    ../../libnetdata/health/health.o \
    ../../libnetdata/json/jsmn.o \
    ../../libnetdata/json/json.o \
    $(NULL)

REGISTRY_OBJS = \
    ../../registry/registry_db.o \
    ../../registry/registry_init.o \
    ../../registry/registry_internals.o \
    ../../registry/registry_log.o \
    ../../registry/registry_machine.o \
    ../../registry/registry_person.o \
    ../../registry/registry_url.o \
    $(NULL)

DATABASE_OBJS = \
    ../../database/rrd.o \
    ../../database/rrdcalc.o \
    ../../database/rrdcalctemplate.o \
    ../../database/rrddim.o \
    ../../database/rrddimvar.o \
    ../../database/rrdfamily.o \
    ../../database/rrdhost.o \
    ../../database/rrdset.o \
    ../../database/rrdsetvar.o \
    ../../database/rrdvar.o \
    $(NULL)

WEB_OBJS = \
    ../../web/api/badges/web_buffer_svg.o \
    ../../web/api/exporters/allmetrics.o \
    ../../web/api/exporters/shell/allmetrics_shell.o \
    ../../web/api/formatters/charts2json.o \
    ../../web/api/formatters/csv/csv.o \
    ../../web/api/formatters/json/json.o \
    ../../web/api/formatters/json_wrapper.o \
    ../../web/api/formatters/rrdset2json.o \
    ../../web/api/formatters/rrd2json.o \
    ../../web/api/formatters/ssv/ssv.o \
    ../../web/api/formatters/value/value.o \
    ../../web/api/health/health_cmdapi.o \
    ../../web/api/queries/average/average.o \
    ../../web/api/queries/des/des.o \
    ../../web/api/queries/incremental_sum/incremental_sum.o \
    ../../web/api/queries/max/max.o \
    ../../web/api/queries/median/median.o \
    ../../web/api/queries/min/min.o \
    ../../web/api/queries/query.o \
    ../../web/api/queries/rrdr.o \
    ../../web/api/queries/ses/ses.o \
    ../../web/api/queries/sum/sum.o \
    ../../web/api/queries/stddev/stddev.o \
    ../../web/api/web_api_v1.o \
    ../../web/server/static/static-threaded.o \
    ../../web/server/web_server.o \
    ../../web/server/web_client.o \
    ../../web/server/web_client_cache.o \
    $(NULL)

STREAMING_OBJS = \
    ../../streaming/rrdpush.o \
    $(NULL)

HEALTH_OBJS = \
    ../../health/health.o \
    ../../health/health_config.o \
    ../../health/health_json.o \
    ../../health/health_log.o \
    $(NULL)

COLLECTOR_OBJS = \
    ../../collectors/plugins.d/plugins_d.o \
    $(NULL)

DAEMON_OBJS = \
    ../../daemon/global_statistics.o \
    $(NULL)

BACKEND_OBJS = \
    ../../backends/backends.o \
    ../../backends/graphite/graphite.o \
    ../../backends/json/json.o \
    ../../backends/opentsdb/opentsdb.o \
    ../../backends/prometheus/backend_prometheus.o \
    $(NULL)

COMMON_LDFLAGS = -pthread -lm -lssl -lcrypto

all: benchmark-dictionary benchmark-line-parsing benchmark-procfile-parser \
     benchmark-registry benchmark-value-pairs statsd-stress test-eval

benchmark-dictionary: benchmark-dictionary.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

benchmark-line-parsing: benchmark-line-parsing.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

benchmark-procfile-parser: benchmark-procfile-parser.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

benchmark-registry: benchmark-registry.c
	gcc ${CFLAGS} -DVARLIB_DIR="\"/var/tmp\"" -o $@ $^ \
	    ${LIBNETDATA_OBJS} ${REGISTRY_OBJS} ${DATABASE_OBJS} ${WEB_OBJS} \
	    ${STREAMING_OBJS} ${HEALTH_OBJS} ${COLLECTOR_OBJS} ${DAEMON_OBJS} \
	    ${BACKEND_OBJS} ${COMMON_LDFLAGS} -luuid -lz

benchmark-value-pairs: benchmark-value-pairs.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

statsd-stress: statsd-stress.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

test-eval: test-eval.c
	gcc ${CFLAGS} -o $@ $^ ${LIBNETDATA_OBJS} ${COMMON_LDFLAGS}

clean:
	rm -f benchmark-dictionary benchmark-line-parsing benchmark-procfile-parser \
	      benchmark-registry benchmark-value-pairs statsd-stress test-eval
