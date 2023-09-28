ARG BUILDER_IMAGE=docker.io/library/golang
ARG BUILDER_VERSION=1.20-bullseye

FROM $BUILDER_IMAGE:$BUILDER_VERSION AS builder

WORKDIR /go/src/app

ENV GOPRIVATE="github.com/atomyze-foundation/*"
ARG NETRC="machine github.com login REGISTRY_USERNAME password REGISTRY_PASSWORD"
ARG VERSION=unknown

RUN echo "$NETRC" > ~/.netrc

COPY go.mod go.sum ./
RUN \
    apt-get update && apt-get install --no-install-recommends -y \
    git=1:2.* \
    upx-ucl=3.* && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

RUN go mod download

COPY . .
RUN go build -trimpath -tags pkcs11 -v -ldflags="-X 'main.AppInfoVer=$VERSION'" -o /go/bin/hlf-control-plane && \
    strip /go/bin/hlf-control-plane && \
    upx -5 -q /go/bin/hlf-control-plane

FROM ubuntu:20.04

ARG YUBIHSM_VERSION="2023-01-ubuntu2004"
ARG YUBIHSM_PACKAGE_VERSION="2.4.0"
ARG YUBIHSM_CONNECTOR_VERSION="3.0.4-1"
ARG SOFTHSM_VERSION="2.5.0-1build1"
ARG CURL_VERSION="7.68.0-1ubuntu2"
ARG LIBUSB_VERSION="2:1.0.23-2build1"
ARG LIBPCSCLITE_VERSION="1.8.26-3"
ARG LIBEDIT_VERSION="3.1-20191231-1"

# install dependencies, softhsm and certs
RUN apt-get update && apt-get install --no-install-recommends -y \
    softhsm2=${SOFTHSM_VERSION} \
    libsofthsm2=${SOFTHSM_VERSION} \
    ca-certificates=2* \
    curl=${CURL_VERSION} \
    libcurl4=${CURL_VERSION} \
    libusb-1.0-0=${LIBUSB_VERSION} \
    libpcsclite1=${LIBPCSCLITE_VERSION} \
    libedit2=${LIBEDIT_VERSION} && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# install yubihsm sdk
WORKDIR /tmp
RUN curl -LO https://developers.yubico.com/YubiHSM2/Releases/yubihsm2-sdk-${YUBIHSM_VERSION}-amd64.tar.gz && \
    tar xzf yubihsm2-sdk-${YUBIHSM_VERSION}-amd64.tar.gz

# set up yubihsm libs
WORKDIR /tmp/yubihsm2-sdk
RUN dpkg -i libyubihsm-http1_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    libyubihsm-usb1_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    libyubihsm1_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-pkcs11_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    libykhsmauth1_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    libyubihsm-dev_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-connector_${YUBIHSM_CONNECTOR_VERSION}_amd64.deb \
    yubihsm-shell_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-auth_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-pkcs11_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-wrap_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    yubihsm-shell_${YUBIHSM_PACKAGE_VERSION}_amd64.deb \
    && rm -rf ../yubihsm2-sdk-${YUBIHSM_VERSION}-amd64.tar.gz ../yubihsm2-sdk && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# add default hsm variables
ENV YUBIHSM_PKCS11_CONF="/etc/yubihsm.conf"
ENV SOFTHSM2_CONF="/etc/softhsm/softhsm2.conf"
# set up working dir and mounted volume with config
VOLUME /etc/control-plane
WORKDIR /etc/control-plane

COPY --chown=65534:65534 --from=builder /go/bin/hlf-control-plane /hlf-control-plane
USER 65534

ENTRYPOINT [ "/hlf-control-plane" ]
