FROM golang:1.26.1-alpine AS builder

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=arm64
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/engine-core ./cmd/engine-core
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/strategy-runner ./cmd/strategy-runner
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/market-ingest ./cmd/market-ingest

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /out/engine-core /app/engine-core
COPY --from=builder /out/strategy-runner /app/strategy-runner
COPY --from=builder /out/market-ingest /app/market-ingest

EXPOSE 8080
ENV ENGINE_CORE_ADDR=:8080
RUN addgroup -S app && adduser -S app -G app
USER app

ENTRYPOINT ["/app/engine-core"]
