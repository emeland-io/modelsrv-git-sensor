FROM golang:1.25 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o modelsrv-git-sensor ./cmd/modelsrv-git-sensor

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/modelsrv-git-sensor .
USER 65532:65532

ENTRYPOINT ["/modelsrv-git-sensor"]
