FROM gentoo/stage3

RUN emerge --sync

RUN emerge dev-vcs/git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

WORKDIR netdata/packaging/installer/

RUN ./kickstart.sh --dont-wait
