# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -o fdo_server ./cmd/fdo_server/

# Start a new stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/fdo_server /app/fdo_server

ENTRYPOINT ["./fdo_server"]
CMD []