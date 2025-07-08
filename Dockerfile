FROM ghcr.io/cybozu/golang:1.24-noble AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/


RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o nyallocator-controller cmd/main.go

FROM scratch
WORKDIR /
COPY --from=builder /workspace/nyallocator-controller .
USER 65532:65532

ENTRYPOINT ["/nyallocator-controller"]
