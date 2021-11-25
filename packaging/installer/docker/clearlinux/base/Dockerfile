FROM clearlinux:base

RUN swupd bundle-add -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/clearlinux.sh

RUN ./netdata/packaging/installer/kickstart.sh
