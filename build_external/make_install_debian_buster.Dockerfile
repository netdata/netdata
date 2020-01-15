FROM debian_buster_dev:latest

# Sanitize new source tree by removing config-time state
COPY . /opt/netdata/latest
WORKDIR /opt/netdata/latest
RUN while read f; do cp -p $f ../source/$f; done <../manifest
WORKDIR /opt/netdata/source
RUN make install
