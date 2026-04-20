FROM node:20-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/admin-api ./cmd/admin-api
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/kline-backfill ./cmd/kline-backfill

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /out/engine-core /app/engine-core
COPY --from=builder /out/strategy-runner /app/strategy-runner
COPY --from=builder /out/market-ingest /app/market-ingest
COPY --from=builder /out/admin-api /app/admin-api
COPY --from=builder /out/kline-backfill /app/kline-backfill
COPY --from=frontend /web/dist /app/web/dist

EXPOSE 8080
ENV ENGINE_CORE_ADDR=:8080
RUN addgroup -S app && adduser -S app -G app
USER app

ENTRYPOINT ["/app/engine-core"]
