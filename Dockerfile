# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder

WORKDIR /go/src/app
COPY . .

RUN apk add --no-cache make curl gcc musl-dev npm
RUN make build && install -D -m 755 go-fdo-server /go/bin/

# Start a new stage
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

RUN apk add --no-cache tzdata curl libecpg

COPY --from=builder /go/bin/go-fdo-server /usr/bin/go-fdo-server

ENTRYPOINT ["go-fdo-server"]
CMD []
