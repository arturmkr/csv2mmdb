FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -ldflags="-s -w" -o /csv2mmdb .

FROM scratch
COPY --from=build /csv2mmdb /csv2mmdb
COPY LICENSE /LICENSE
USER 65532:65532
ENTRYPOINT ["/csv2mmdb"]
