# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache 2.0

FROM golang:1.25-alpine AS builder

WORKDIR /go/src/app
COPY . .

RUN apk add make curl
RUN make
RUN install -D -m 755 go-fdo-server /go/bin/
RUN <<EOF
  set -xeuo pipefail
  latest_version_url=`curl --fail --output /dev/null --silent --write-out '%{redirect_url}' https://github.com/stunnel/static-curl/releases/latest`
  latest_version_path=${latest_version_url/*\//}
  latest_version=${latest_version_path%-ech}
  arch=`uname -m`
  curl -sLO https://github.com/stunnel/static-curl/releases/download/${latest_version_path}/curl-linux-${arch}-glibc-${latest_version}.tar.xz
  tar -xf curl-linux-${arch}-glibc-${latest_version}.tar.xz -C /go/bin curl
EOF

# Start a new stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /go/bin/go-fdo-server /usr/bin/go-fdo-server
COPY --from=builder /go/bin/curl /usr/bin/curl

ENTRYPOINT ["go-fdo-server"]
CMD []
