FROM fedora:30
RUN dnf -y update
RUN dnf install -y autoconf automake curl gcc git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel make nc pkgconfig python36 zlib-devel libwebsockets-devel findutils
RUN curl -L https://downloads.sourceforge.net/project/judy/judy/Judy-1.0.5/Judy-1.0.5.tar.gz -o /opt/judy-1.0.5.tgz
RUN cd /opt && tar xzf judy-1.0.5.tgz && cd judy-1.0.5 && ./configure && make && make install
RUN cp /usr/local/lib/libJudy* /lib64/

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
    -D_FORTIFY_SOURCE=2 -DNETDATA_VERIFY_LOCKS=1" ./netdata-installer.sh --disable-lto

RUN ln -sf /dev/stdout /var/log/netdata/access.log
RUN ln -sf /dev/stdout /var/log/netdata/debug.log
RUN ln -sf /dev/stderr /var/log/netdata/error.log
