FROM gcc:8

RUN apt-get update && apt-get install -y \
    autoconf-archive \
    autogen \
    libmnl-dev \
    uuid-dev \
    rpm \
    rubygems \
    ruby-dev \
 && rm -rf /var/lib/apt/lists/* \
 && gem install fpm
