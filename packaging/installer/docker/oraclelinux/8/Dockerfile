FROM oraclelinux:8

RUN dnf config-manager --set-enabled ol8_codeready_builder 

RUN dnf makecache --refresh

RUN dnf install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/ol.sh

RUN ./netdata/packaging/installer/kickstart.sh
