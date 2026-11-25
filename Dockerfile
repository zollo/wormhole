# ---- build stage ----
FROM golang:1.24-alpine AS build

WORKDIR /src

# Cache dependencies first. Wormhole has none beyond the standard library,
# but this keeps rebuilds fast if that ever changes.
COPY go.mod ./
RUN go mod download

COPY . .

ARG VERSION=docker
# CGO disabled + static build so the binary runs in a scratch image.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/wormhole .

# ---- runtime stage ----
FROM scratch

# Bring CA certificates along so HTTPS origins can be dialled for CONNECT.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/wormhole /wormhole

EXPOSE 8080

# Run as a non-root user id.
USER 65534:65534

ENTRYPOINT ["/wormhole"]
