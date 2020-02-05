FROM ubuntu:20.04

RUN apt-get update
RUN apt-get install -y zlib1g-dev uuid-dev libuv1-dev liblz4-dev libjudy-dev libssl-dev libmnl-dev gcc \
    make git autoconf autoconf-archive autogen automake pkg-config curl python2

COPY . /opt/netdata/source
WORKDIR /opt/netdata/source

# RUN make distclean   -> not safe if tree state changed on host since last config
# Kill everything that is not in .gitignore preserving any fresh changes
RUN git config --global user.email "root@container"
RUN git config --global user.name "Fake root"
RUN git stash && git clean -dxf && (git stash apply || true)
# Not everybody is updating distclean properly - fix.
RUN find . -name '*.Po' -exec rm \{\} \;
RUN rm -rf autom4te.cache
RUN rm -rf .git/
RUN find . -type f >/opt/netdata/manifest

RUN CFLAGS="-O1 -ggdb -Wall -Wextra -Wformat-signedness -fstack-protector-all -DNETDATA_INTERNAL_CHECKS=1\
    -D_FORTIFY_SOURCE=2 -DNETDATA_VERIFY_LOCKS=1" ./netdata-installer.sh --disable-lto

RUN ln -sf /dev/stdout /var/log/netdata/access.log
RUN ln -sf /dev/stdout /var/log/netdata/debug.log
RUN ln -sf /dev/stderr /var/log/netdata/error.log


