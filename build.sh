#!/bin/bash

set -e

VERSION=$(cat VERSION)
IMAGE="bamboo-v${VERSION}"
HOST="catalog.shurenyun.com"
NAMESPACE="library"
TAG="omega.v2.5.9"

docker build --no-cache -t ${HOST}/${NAMESPACE}/${IMAGE}:${TAG} -f Dockerfile .
