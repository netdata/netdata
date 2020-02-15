FROM archlinux/base:latest

RUN pacman -Sy
RUN pacman --noconfirm --needed -S autoconf \
                                   autoconf-archive \
                                   autogen \
                                   automake \
                                   gcc \
                                   make \
                                   git \
                                   libuv \
                                   lz4 \
                                   netcat \
                                   openssl \
                                   pkgconfig \
                                   python \
                                   libvirt \
                                   libwebsockets

ARG ACLK=no
ARG EXTRA
COPY . /opt/netdata/source
WORKDIR /opt/netdata/source

RUN git config --global user.email "root@container"
RUN git config --global user.name "Fake root"

# RUN make distclean   -> not safe if tree state changed on host since last config
# Kill everything that is not in .gitignore preserving any fresh changes, i.e. untracked changes will be
# deleted but local changes to tracked files will be preserved.
RUN if git status --porcelain | grep '^[MADRC]'; then \
        git stash && git clean -dxf && (git stash apply || true) \
    else \
        git clean -dxf ; \
    fi

# Not everybody is updating distclean properly - fix.
RUN find . -name '*.Po' -exec rm \{\} \;
RUN rm -rf autom4te.cache
RUN rm -rf .git/
RUN find . -type f >/opt/netdata/manifest

RUN CFLAGS="-O1 -ggdb -Wall -Wextra -Wformat-signedness -fstack-protector-all -DNETDATA_INTERNAL_CHECKS=1\
    -D_FORTIFY_SOURCE=2 -DNETDATA_VERIFY_LOCKS=1 ${EXTRA}" ./netdata-installer.sh --disable-lto

RUN ln -sf /dev/stdout /var/log/netdata/access.log
RUN ln -sf /dev/stdout /var/log/netdata/debug.log
RUN ln -sf /dev/stderr /var/log/netdata/error.log
