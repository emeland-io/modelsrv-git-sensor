FROM golang:1.25 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o modelsrv-git-sensor ./cmd/modelsrv-git-sensor

FROM alpine:3.21
RUN apk add --no-cache git openssh-client
WORKDIR /
COPY --from=builder /workspace/modelsrv-git-sensor .
USER nobody

ENTRYPOINT ["/modelsrv-git-sensor"]
