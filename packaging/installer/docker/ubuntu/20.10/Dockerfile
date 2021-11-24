FROM ubuntu:20.10

RUN apt-get update

RUN apt-get install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/debian.sh

RUN ./netdata/packaging/installer/kickstart.sh
