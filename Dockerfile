FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

ARG TARGETOS
ARG TARGETARCH
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /manager ./cmd/manager
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /agent ./cmd/agent

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /manager /manager
COPY --from=builder /agent /agent
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
USER 65532:65532
ENTRYPOINT ["/manager"]
