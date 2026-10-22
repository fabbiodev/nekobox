# Void Linux (glibc)
FROM ghcr.io/void-linux/void-glibc:latest

ARG TARGETPLATFORM
ARG BUILDPLATFORM

RUN echo "FINE"
RUN echo "I am running on $BUILDPLATFORM, building for $TARGETPLATFORM"

# Update and install in one retried transaction. Void mirrors can briefly return
# 404 while per-architecture repository indexes are being synchronized.
RUN set -eux; \
    installed=0; \
    for mirror in \
      https://repo-fastly.voidlinux.org \
      https://repo-default.voidlinux.org; do \
      sed -i "s|https://repo-[^/]\\+\\.voidlinux\\.org|$mirror|g" /usr/share/xbps.d/*-repository-*.conf; \
      if xbps-install -Syu -y \
        bash cmake git ninja jq curl ccache wget sed util-linux coreutils shadow squashfs-tools \
        qt6-base-devel thrift-devel openssl-devel boost-devel lmdb-devel acl-devel libunistring-devel \
        libcurl-devel libzstd-devel patchelf pkg-config qt6-base qt6-tools thrift base-devel \
        tar xz which file openssh yaml-cpp-devel leveldb-devel; then \
        installed=1; \
        break; \
      fi; \
    done; \
    test "$installed" = 1

# The current sing-box dependency uses encoding/json/v2 names removed in Go
# 1.27. Pin the Linux builder to the tested Go 1.26.10 toolchain on every arch.
RUN set -eux; \
    case "$TARGETPLATFORM" in \
      linux/amd64) go_arch=amd64; go_sha=ccfe7ae822d2c2f41f4fe7c956199967a4dac22d0ee0d2b2bfdb440aab3b20c5 ;; \
      linux/arm64) go_arch=arm64; go_sha=3f4227ce7b206a513e794f1393adb473a07b46e94d18bd15d1c684bbb7319797 ;; \
      linux/386) go_arch=386; go_sha=fa05f9c9d71d08da81ae34f1b6baf8aacb05374b9a6fb223e92f7b29afc81148 ;; \
      linux/arm/v7) go_arch=armv6l; go_sha=7d89fd32259eb0ff647381e8c3465dc32ccfc7f71cb7b9bcd95604c3481f5109 ;; \
      *) echo "Unsupported Go build platform: $TARGETPLATFORM" >&2; exit 1 ;; \
    esac; \
    curl --fail --location --retry 4 --retry-all-errors \
      "https://go.dev/dl/go1.26.10.linux-${go_arch}.tar.gz" \
      --output /tmp/go1.26.10.tar.gz; \
    echo "${go_sha}  /tmp/go1.26.10.tar.gz" | sha256sum -c -; \
    tar -C /opt -xzf /tmp/go1.26.10.tar.gz; \
    rm /tmp/go1.26.10.tar.gz

ENV PATH="/opt/go/bin:${PATH}"
RUN go version

WORKDIR /app

CMD ["bash", "-x", "/app/script/build_docker.sh"]
