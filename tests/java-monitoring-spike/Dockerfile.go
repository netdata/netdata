ARG ARTIFACT_IMAGE=netdata-java-spike:monitor
FROM ${ARTIFACT_IMAGE} AS artifacts
FROM netdata/netdata@sha256:2dd6963cb15637748985871016af3c52d1a0cc67ea03a5b7fcfe51600e481e9f
COPY --from=artifacts /lab/ /lab/
COPY --from=artifacts /opt/java/openjdk/ /lab/jdk/
COPY javaspike.plugin /lab/javaspike.bin
COPY plugin-proxy /usr/libexec/netdata/plugins.d/javaspike.plugin
RUN mkdir -p /state /etc/netdata/javaspike && chmod 700 /state
ENTRYPOINT ["/usr/sbin/netdata", "-D", "-u", "root"]
