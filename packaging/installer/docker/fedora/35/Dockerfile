FROM fedora:35

RUN dnf install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/fedora.sh

RUN ./netdata/packaging/installer/kickstart.sh
