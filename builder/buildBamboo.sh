#!/bin/bash

export GOROOT=/usr/lib/go
export GOPATH=/gopath
export GOBIN=/gopath/bin
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin

cd /gopath/src/github.com/QubitProducts/bamboo
go get github.com/tools/godep && \
go get -t github.com/smartystreets/goconvey && \
go build && \
mkdir -p /var/bamboo && \
cp  /gopath/src/github.com/QubitProducts/bamboo/bamboo /var/bamboo/bamboo && \
mkdir -p /var/log/supervisor

rm -rf /tmp/* /var/tmp/*
rm -f /etc/ssh/ssh_host_*
rm -rf /gopath
rm -rf /usr/lib/go
