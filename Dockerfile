FROM --platform=$BUILDPLATFORM golang:1.18.3-alpine3.16 AS build

WORKDIR /usr/local/go/src/truenas-scale-csi

COPY go.mod go.sum /usr/local/go/src/truenas-scale-csi/

RUN go mod download

COPY cmd/ /usr/local/go/src/truenas-scale-csi/cmd/
COPY pkg/ /usr/local/go/src/truenas-scale-csi/pkg/

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /plugin cmd/truenas-csi-plugin/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /iscsiadm cmd/iscsiadm/main.go

FROM alpine:3.16.0 AS release
RUN apk add --no-cache lsblk=2.38-r1 e2fsprogs=1.46.5-r0 xfsprogs=5.16.0-r1 util-linux-misc=2.38-r1
# lsblk
# e2fsprogs -> mkfs.ext3, mkfs.ext4, fsck.ext3, fsck.ext4
# xfsprogs -> mkfs.xfs, fsck.xfs
# util-linux-misc -> mount

COPY --from=build /plugin /plugin
COPY --from=build /iscsiadm /sbin/iscsiadm

ENTRYPOINT ["/plugin"]