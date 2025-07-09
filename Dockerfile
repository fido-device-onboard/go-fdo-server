# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.23-alpine AS builder

WORKDIR /go/src/app
COPY . .

RUN apk add make
RUN make BINARY_NAME=/go/bin/go-fdo-server

# Start a new stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /go/bin/go-fdo-server /usr/bin/go-fdo-server

ENTRYPOINT ["go-fdo-server"]
CMD []
