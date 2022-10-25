# Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
# Use of this source code is covered by the following dual licenses:
# (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
# (2) Affero General Public License 3.0 (AGPL 3.0)
# license that can be found in the LICENSE file.

# Build the manager binary
FROM hub.qucheng.com/library/god as builder

ENV GOPROXY=https://goproxy.cn,direct

ENV WORKDIR /go/src/github.com/easysoft/qucheng-operator
WORKDIR $WORKDIR
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY apis/ apis/
COPY pkg/ pkg/
COPY controllers/ controllers/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o cne-operator ./cmd/ && cp cne-operator /usr/bin/cne-operator

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM hub.qucheng.com/app/mysql:5.7.37-debian-10 as mysql57

FROM hub.qucheng.com/app/postgresql:14.3.0-debian-10-r13 as pg14

FROM hub.qucheng.com/library/debian:11.3-slim
WORKDIR /

COPY docker/prebuildfs /

ENV TZ=Asia/Shanghai \
    DEBIAN_FRONTEND=noninteractive

RUN sed -i -r 's/(deb|security).debian.org/mirrors.cloud.tencent.com/g' /etc/apt/sources.list \
    && install_packages curl tzdata apt-transport-https ca-certificates procps \
    && ln -fs /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo ${TZ} > /etc/timezone \
    && dpkg-reconfigure --frontend noninteractive tzdata

# mysql binary and lib
COPY --from=mysql57 /opt/bitnami/mysql/bin/mysql /bin/mysql
COPY --from=mysql57 /opt/bitnami/mysql/bin/mysqldump /bin/mysqldump
COPY --from=mysql57 /lib/x86_64-linux-gnu/libncurses.so.6 /lib/x86_64-linux-gnu/libncurses.so.6
COPY --from=mysql57 /usr/lib/x86_64-linux-gnu/libatomic.so.1 /usr/lib/x86_64-linux-gnu/libatomic.so.1


# postgresql binary and lib
COPY --from=pg14 /opt/bitnami/postgresql/bin/psql /bin/psql
COPY --from=pg14 /opt/bitnami/postgresql/bin/pg_dump /bin/pg_dump
COPY --from=pg14 /opt/bitnami/postgresql/bin/pg_restore /bin/pg_restore
COPY --from=pg14 /opt/bitnami/postgresql/lib/libpq.so.5.14 /lib/x86_64-linux-gnu/libpq.so.5
COPY --from=pg14 /usr/lib/x86_64-linux-gnu/libedit.so.2.0.59 /lib/x86_64-linux-gnu/libedit.so.2

# copy restic
COPY --from=hub.qucheng.com/third-party/restic:0.13.1 /usr/bin/restic /usr/bin/restic

ADD config/crd /opt/crd

# copy build asset
COPY --from=builder /usr/bin/cne-operator /usr/bin/cne-operator

USER 65534:65534

ENTRYPOINT ["/usr/bin/cne-operator", "--crd-path=/opt/crd/bases"]
