# Static binary on distroless: the oEmbed provider list is embedded, so the image is the binary and its license
# notices. Go cross-compiles for every target platform on the build host, so a multi-arch image needs no emulation.

FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
# VERSION is what /healthz, the OpenAPI document and the User-Agent report, such as what git describe says; the build
# context has no .git to read it from. Empty reports devel.
ARG VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/pantafive/embisto.release=${VERSION}" -o /out/embisto ./cmd/embisto
RUN go run ./scripts/notices -o /out/THIRD_PARTY_NOTICES

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/LICENSE /out/THIRD_PARTY_NOTICES /usr/share/doc/embisto/
COPY --from=build /out/embisto /embisto
USER nonroot:nonroot
# Only the API: the internal listener on 9090 serves the purge and must never be published.
EXPOSE 8080
ENTRYPOINT ["/embisto"]
