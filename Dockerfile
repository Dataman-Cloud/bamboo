FROM catalog.shurenyun.com/library/haproxy:1.5.18-alpine
MAINTAINER Xiao Deshi <dsxiao@dataman-inc.com>

RUN mkdir -p /config

ADD config/haproxy_template.cfg /config/haproxy_template.cfg
ADD config/development.json /config/development.json

ADD . /gopath/src/github.com/QubitProducts/bamboo
ADD haproxy /usr/share/haproxy
ADD builder/supervisord.conf /etc/supervisord.conf
ADD builder/run.sh /run.sh
ADD builder/buildBamboo.sh /buildBamboo.sh
WORKDIR /

RUN sh /buildBamboo.sh

EXPOSE 5090

CMD sh /run.sh
