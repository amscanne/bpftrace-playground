ARG GO_VERSION=1.23
FROM golang:${GO_VERSION} AS builder
ARG TARGETOS
ARG TARGETARCH
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bpftrace-playground playground.go
    
FROM gcr.io/distroless/static-debian12
COPY --from=builder /bpftrace-playground /
WORKDIR /
ENTRYPOINT ["/bpftrace-playground"]
