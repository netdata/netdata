FROM archlinux:base

RUN pacman -Sy --noconfirm git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

WORKDIR /netdata/packaging/installer

RUN ./kickstart.sh --dont-wait
