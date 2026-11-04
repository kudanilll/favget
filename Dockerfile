FROM golang:1.27-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /favget \
    ./cmd/server

FROM alpine:3.24

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

COPY --from=builder /favget /usr/local/bin/favget

USER appuser

EXPOSE 8080

ENV APP_ENV=production
ENV PORT=8080
ENV TZ=Asia/Jakarta

CMD ["favget"]
