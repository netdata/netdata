FROM ubuntu:21.10

RUN apt-get update

RUN apt-get install -y tzdata

RUN apt-get install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

WORKDIR /netdata/packaging/installer

RUN ./kickstart.sh --dont-wait
