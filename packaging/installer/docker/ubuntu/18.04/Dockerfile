FROM ubuntu:18.04

RUN apt-get update

RUN apt-get install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/ubuntu.sh

RUN ./netdata/packaging/installer/kickstart.sh
