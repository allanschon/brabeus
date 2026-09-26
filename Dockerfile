# Build in a container so no host needs a Go toolchain to keep patched.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/brabeus .

# Not distroless: the write path shells out to git over ssh, so the runtime
# needs both. Alpine keeps that to a few megabytes.
FROM alpine:3.21
RUN apk add --no-cache git openssh-client ca-certificates tini
COPY --from=build /out/brabeus /usr/local/bin/brabeus

# Runs as root deliberately: the deploy key is root:root 600 on the host
# and is bind-mounted read-only. Giving the key a second owner to satisfy a
# non-root container would widen who can read it on the host, which is the
# wrong trade for a Tailscale-only service.
ENV BRABEUS_LISTEN=0.0.0.0:8082
EXPOSE 8082
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/brabeus"]
