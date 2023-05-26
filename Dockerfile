FROM --platform=$BUILDPLATFORM golang:1.20.4-alpine3.18 AS build

WORKDIR /usr/local/go/src/truenas-scale-csi

COPY go.mod go.sum /usr/local/go/src/truenas-scale-csi/

RUN go mod download

COPY cmd/ /usr/local/go/src/truenas-scale-csi/cmd/
COPY pkg/ /usr/local/go/src/truenas-scale-csi/pkg/

ENV PKG=github.com/terrycain/truenas-scale-csi
ARG DOCKER_METADATA_OUTPUT_JSON

# hadolint ignore=DL3018,SC2086,DL4006,SC2155
RUN apk add --no-cache jq && \
    export GIT_COMMIT="$(echo "${DOCKER_METADATA_OUTPUT_JSON}" | jq -r '.labels["org.opencontainers.image.revision"]')" && \
    export BUILD_DATE="$(echo "${DOCKER_METADATA_OUTPUT_JSON}" | jq -r '.labels["org.opencontainers.image.created"]')" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -o /plugin \
      -ldflags "-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w" \
      cmd/truenas-csi-plugin/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /iscsiadm cmd/iscsiadm/main.go

FROM alpine:3.18.0 AS release
RUN apk add --no-cache lsblk=2.38.1-r7 e2fsprogs=1.47.0-r2 xfsprogs=6.2.0-r2 util-linux-misc=2.38.1-r7 nfs-utils=2.6.3-r1 blkid=2.38.1-r7
# lsblk
# blkid
# e2fsprogs -> mkfs.ext3, mkfs.ext4, fsck.ext3, fsck.ext4
# xfsprogs -> mkfs.xfs, fsck.xfs
# util-linux-misc -> mount
# nfs-utils -> mount.nfs, showmount

COPY --from=build /plugin /plugin
COPY --from=build /iscsiadm /sbin/iscsiadm

ENTRYPOINT ["/plugin"]