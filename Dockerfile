FROM golang:1.26-alpine3.23 AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod tidy

COPY . .
RUN go build -o botdiscord ./cmd/bot

FROM alpine:3.23

RUN apk add --no-cache ffmpeg opus  
RUN wget -q https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -O /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp

WORKDIR /app
COPY --from=builder /app/botdiscord .
COPY .env.example .env

CMD ["./botdiscord"]
