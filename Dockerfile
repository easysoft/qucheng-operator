# Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
# Use of this source code is covered by the following dual licenses:
# (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
# (2) Affero General Public License 3.0 (AGPL 3.0)
# license that can be found in the LICENSE file.

# Build the manager binary
FROM hub.qucheng.com/library/god as builder

ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /workspace
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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o cne-operator cmd/main.go

RUN upx -q cne-operator

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM hub.qucheng.com/app/mysql:5.7.37-debian-10 as mysql57

FROM hub.qucheng.com/library/debian:11.3-slim
WORKDIR /

COPY --from=mysql57 /opt/bitnami/mysql/bin/mysql /bin/mysql
COPY --from=mysql57 /opt/bitnami/mysql/bin/mysqldump /bin/mysqldump
COPY --from=mysql57 /lib/x86_64-linux-gnu/libncurses.so.6 /lib/x86_64-linux-gnu/libncurses.so.6
COPY --from=mysql57 /usr/lib/x86_64-linux-gnu/libatomic.so.1 /usr/lib/x86_64-linux-gnu/libatomic.so.1

COPY --from=builder /workspace/cne-operator .

USER 65534:65534

ENTRYPOINT ["/cne-operator"]
