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
# 1.27. Pin the Linux builder to the tested Go 1.26.1 toolchain on every arch.
RUN set -eux; \
    case "$TARGETPLATFORM" in \
      linux/amd64) go_arch=amd64; go_sha=031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a ;; \
      linux/arm64) go_arch=arm64; go_sha=a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7 ;; \
      linux/386) go_arch=386; go_sha=da75d696c6b9440fe9fb6418429f29eaeee947707ee8c6ddb567c558051a1cc2 ;; \
      linux/arm/v7) go_arch=armv6l; go_sha=c9937198994dc173b87630a94a0d323442bef81bf7589b1170d55a8ebf759bda ;; \
      *) echo "Unsupported Go build platform: $TARGETPLATFORM" >&2; exit 1 ;; \
    esac; \
    curl --fail --location --retry 4 --retry-all-errors \
      "https://go.dev/dl/go1.26.1.linux-${go_arch}.tar.gz" \
      --output /tmp/go1.26.1.tar.gz; \
    echo "${go_sha}  /tmp/go1.26.1.tar.gz" | sha256sum -c -; \
    tar -C /opt -xzf /tmp/go1.26.1.tar.gz; \
    rm /tmp/go1.26.1.tar.gz

ENV PATH="/opt/go/bin:${PATH}"
RUN go version

WORKDIR /app

CMD ["bash", "-x", "/app/script/build_docker.sh"]
