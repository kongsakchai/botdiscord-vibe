FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o botdiscord ./cmd/bot

FROM alpine:3.19

RUN apk add --no-cache ffmpeg libopus
RUN wget -q https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -O /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp

WORKDIR /app
COPY --from=builder /app/botdiscord .
COPY .env.example .env

CMD ["./botdiscord"]