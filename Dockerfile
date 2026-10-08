# Anam as a service: `anam daemon` on a distroless base, as a non-root user.
#
# The build context is the repo, but .dockerignore admits only what the binary is
# built from: no private/, no exports, no ledger copies. modernc.org/sqlite is pure
# Go, so the binary is static and the base needs nothing but certificates.
#
# Version identity comes in as build args from scripts/version.sh, as the Makefile
# passes it in ldflags; "dev" is the honest fallback.

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /out/anam ./cmd/anam

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/anam /usr/local/bin/anam
USER nonroot:nonroot
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/anam", "daemon", "ready"]
ENTRYPOINT ["/usr/local/bin/anam"]
CMD ["daemon"]
