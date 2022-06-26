FROM --platform=$BUILDPLATFORM golang:1.18.3-alpine3.16 AS build

WORKDIR /usr/local/go/src/truenas-scale-csi

COPY go.mod go.sum /usr/local/go/src/cmd/truenas-scale-csi/

RUN go mod download

ADD cmd/ /usr/local/go/src/cmd/qnap-csi-plugin/cmd/
ADD pkg/ /usr/local/go/src/cmd/qnap-csi-plugin/pkg/

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /plugin cmd/truenas-csi-plugin/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /iscsiadm cmd/iscsiadm/main.go

FROM alpine:3.16.0 AS release
RUN apk update && \
    apk add lsblk e2fsprogs xfsprogs util-linux-misc
# lsblk
# e2fsprogs -> mkfs.ext3, mkfs.ext4, fsck.ext3, fsck.ext4
# xfsprogs -> mkfs.xfs, fsck.xfs
# util-linux-misc -> mount
COPY --from=build /plugin /plugin
COPY --from=build /iscsiadm /sbin/iscsiadm

ENTRYPOINT ["/plugin"]