# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.25-alpine AS builder

WORKDIR /go/src/app
COPY . .

RUN apk add make curl gcc musl-dev
RUN make build && install -D -m 755 go-fdo-server /go/bin/

# Start a new stage
FROM alpine

RUN apk add tzdata curl libecpg

COPY --from=builder /go/bin/go-fdo-server /usr/bin/go-fdo-server

ENTRYPOINT ["go-fdo-server"]
CMD []
