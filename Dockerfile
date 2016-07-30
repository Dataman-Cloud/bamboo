FROM catalog.shurenyun.com/library/haproxy:1.5.18-alpine
MAINTAINER Xiao Deshi <dsxiao@dataman-inc.com>

RUN mkdir -p /config

RUN apk update && \
    apk add  --no-cache curl git bash go supervisor net-tools && rm -rf /var/cache/apk/*

ADD config/haproxy_template.cfg /config/haproxy_template.cfg
ADD config/production.json /config/production.json

ADD . /gopath/src/github.com/QubitProducts/bamboo
ADD haproxy /usr/share/haproxy
ADD builder/supervisord.conf /etc/supervisord.conf
ADD builder/run.sh /run.sh
ADD builder/buildBamboo.sh /buildBamboo.sh
WORKDIR /

RUN sh /buildBamboo.sh

EXPOSE 5090

CMD sh /run.sh
