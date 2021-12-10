FROM clearlinux:base

RUN swupd bundle-add -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

WORKDIR /netdata/packaging/installer

RUN ./kickstart.sh --dont-wait
