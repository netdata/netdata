# java-package is a named build context containing a standard-prefix staged
# installation built with ENABLE_PLUGIN_JAVA=ON for this image's architecture.
ARG ARTIFACT_IMAGE=netdata-java-spike:monitor
FROM ${ARTIFACT_IMAGE} AS artifacts
FROM netdata/netdata@sha256:2dd6963cb15637748985871016af3c52d1a0cc67ea03a5b7fcfe51600e481e9f
USER root
COPY --from=artifacts /lab/ /lab/
COPY --from=java-package /usr/libexec/netdata/plugins.d/java.plugin /lab/java.bin
COPY --from=java-package /usr/libexec/netdata/plugins.d/java-helper /usr/libexec/netdata/plugins.d/java-helper
COPY --from=java-package /usr/libexec/netdata/plugins.d/ndsudo /usr/libexec/netdata/plugins.d/ndsudo
COPY --from=java-package /usr/share/netdata/java/ /usr/share/netdata/java/
COPY --from=java-package /usr/lib/netdata/conf.d/java.conf /usr/lib/netdata/conf.d/java.conf
COPY --from=java-package /usr/lib/netdata/conf.d/java/ /usr/lib/netdata/conf.d/java/
COPY plugin-proxy /usr/libexec/netdata/plugins.d/java.plugin
RUN mkdir -p /state /fixtures /etc/netdata/java /var/lib/netdata && \
    chown netdata:netdata /state /var/lib/netdata && chmod 700 /state && \
    chown 10001:10001 /fixtures && chmod 700 /fixtures && \
    chown root:netdata /usr/libexec/netdata/plugins.d/ndsudo && \
    chmod 4750 /usr/libexec/netdata/plugins.d/ndsudo && \
    chmod 755 /usr/libexec/netdata/plugins.d/java.plugin /usr/libexec/netdata/plugins.d/java-helper /lab/java.bin && \
    /usr/share/netdata/java/runtime/bin/java -version
ENTRYPOINT ["/usr/sbin/netdata", "-D", "-u", "netdata"]
