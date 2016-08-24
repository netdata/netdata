FROM debian:sid

RUN DEBIAN_FRONTEND=noninteractive apt-get -qq update
RUN DEBIAN_FRONTEND=noninteractive apt-get -qq -y install zlib1g-dev gcc make git autoconf autogen automake pkg-config  

RUN git clone https://github.com/firehol/netdata.git netdata.git --depth=1 && \
   cd netdata.git && \
   ./netdata-installer.sh

EXPOSE 19999

ENTRYPOINT ["/usr/sbin/netdata", "-nd"]
