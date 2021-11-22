FROM alpine:3.13

RUN apk add git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/alpine.sh

RUN ./netdata/packaging/installer/kickstart.sh
