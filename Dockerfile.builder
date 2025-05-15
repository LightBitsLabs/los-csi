FROM golang:1.24.3-alpine3.21 AS builder

ARG GID=1000
ARG UID=1000
ARG DOCKER_GID=998
ARG HELM_VERSION

RUN apk update && \
    apk add --no-cache \
    curl \
    docker-cli \
    bash \
    make \
    git \
    g++

RUN curl -fsSL -o /tmp/get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 && \
    chmod 700 /tmp/get_helm.sh && \
    VERIFY_CHECKSUM=false /tmp/get_helm.sh --version $HELM_VERSION && \
    rm /tmp/get_helm.sh

ENV UNAME=app

RUN addgroup -g $GID $UNAME
# on alpine ping group is coliding with docker-group on id 999 - delete it if exists
RUN getent group ping && delgroup ping
RUN addgroup -g $DOCKER_GID docker
RUN adduser --disabled-password -u $UID -G $UNAME -G docker $UNAME
USER $UNAME
