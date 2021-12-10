FROM alpine:edge

RUN apk add git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN /netdata/packaging/installer

RUN ./kickstart.sh --dont-wait
