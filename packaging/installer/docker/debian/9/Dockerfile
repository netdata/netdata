FROM debian:9

RUN apt-get update

RUN apt-get install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN /netdata/packaging/installer

RUN ./kickstart.sh --dont-wait
