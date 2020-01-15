FROM ubuntu:20.04
RUN apt-get update
RUN apt-get install -y zlib1g-dev uuid-dev libuv1-dev liblz4-dev libjudy-dev libssl-dev libmnl-dev gcc make git autoconf autoconf-archive autogen automake pkg-config curl python
 
