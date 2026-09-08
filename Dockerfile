FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ ca-certificates curl && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN bash scripts/fetch-ntgcalls.sh
ENV CGO_ENABLED=1
RUN go build -trimpath -ldflags="-s -w" -o /out/musicbot ./cmd/musicbot

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg && \
    curl --fail --location --retry 3 https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux -o /usr/local/bin/yt-dlp && \
    chmod 0755 /usr/local/bin/yt-dlp && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /out/musicbot /app/musicbot
COPY --from=builder /src/libs/lib/libntgcalls.so /app/libs/lib/libntgcalls.so
ENV LD_LIBRARY_PATH=/app/libs/lib
CMD ["/app/musicbot"]
