# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build

# Start a new stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/go-fdo-server /app/go-fdo-server

ENTRYPOINT ["./go-fdo-server"]
CMD []