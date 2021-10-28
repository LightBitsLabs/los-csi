FROM golang:1.17.2-alpine3.14 AS builder

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
    VERIFY_CHECKSUM=false /tmp/get_helm.sh --version v3.7.1 && \
    rm /tmp/get_helm.sh

ENV UNAME=app

ARG GID=1000
ARG UID=1000
ARG DOCKER_GID=998

RUN echo $UNAME
RUN echo $UID
RUN echo $GID
RUN echo $DOCKER_GID

RUN addgroup -g $GID $UNAME
RUN addgroup -g $DOCKER_GID docker
RUN adduser --disabled-password -u $UID -G $UNAME -G docker $UNAME
USER $UNAME
