# syntax=docker/dockerfile:1

# The toolchain runs on the machine doing the building and cross-compiles from there.
FROM --platform=$BUILDPLATFORM golang:latest AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# The sources and the assets embedded in the executable.
COPY *.go ./
COPY assets ./assets

# The platform the image is built for, named by the builder.
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/serve-markdown .

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/serve-markdown /usr/local/bin/serve-markdown

# The documents are served from what is mounted here.
WORKDIR /data
EXPOSE 8000

# The container is reached from outside its own loopback.
ENTRYPOINT ["/usr/local/bin/serve-markdown", "--listen-address", "0.0.0.0"]
CMD []
