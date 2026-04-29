# syntax=docker/dockerfile:1.7

FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/dns-image-gen .

FROM alpine:3.20
WORKDIR /app

COPY --from=builder /out/dns-image-gen /app/dns-image-gen
COPY g24-left.png g24-right.png g24-center.png normal.png medium.png rare.png super.png JetBrainsMono-ExtraBold.ttf /app/

EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/dns-image-gen", "--prefix=/app", "--host=0.0.0.0:8080"]
