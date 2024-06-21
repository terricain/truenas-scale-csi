FROM --platform=$BUILDPLATFORM golang:1.22.4-alpine3.20 AS build

WORKDIR /usr/local/go/src/truenas-scale-csi

COPY go.mod go.sum /usr/local/go/src/truenas-scale-csi/

RUN go mod download

COPY cmd/ /usr/local/go/src/truenas-scale-csi/cmd/
COPY pkg/ /usr/local/go/src/truenas-scale-csi/pkg/

ENV PKG=github.com/terricain/truenas-scale-csi
ARG DOCKER_METADATA_OUTPUT_JSON
ARG TARGETOS
ARG TARGETARCH

# hadolint ignore=DL3018,SC2086,DL4006,SC2155
RUN apk add --no-cache jq && \
    export VERSION="$(echo "${DOCKER_METADATA_OUTPUT_JSON}" | jq -r '.labels["org.opencontainers.image.version"]')" && \
    export GIT_COMMIT="$(echo "${DOCKER_METADATA_OUTPUT_JSON}" | jq -r '.labels["org.opencontainers.image.revision"]')" && \
    export BUILD_DATE="$(echo "${DOCKER_METADATA_OUTPUT_JSON}" | jq -r '.labels["org.opencontainers.image.created"]')" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -o /plugin \
      -ldflags "-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w" \
      cmd/truenas-csi-plugin/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /iscsiadm cmd/iscsiadm/main.go

FROM alpine:3.20 AS release
RUN apk add --no-cache lsblk=2.40.1-r1 e2fsprogs=1.47.0-r5 xfsprogs=6.8.0-r0 util-linux-misc=2.40.1-r1 nfs-utils=2.6.4-r1 blkid=2.40.1-r1
# lsblk
# blkid
# e2fsprogs -> mkfs.ext3, mkfs.ext4, fsck.ext3, fsck.ext4
# xfsprogs -> mkfs.xfs, fsck.xfs
# util-linux-misc -> mount
# nfs-utils -> mount.nfs, showmount

COPY --from=build /plugin /plugin
COPY --from=build /iscsiadm /sbin/iscsiadm

ENTRYPOINT ["/plugin"]
