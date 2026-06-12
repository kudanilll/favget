# Build stage
FROM golang:1.25.4-alpine3.21 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags='-s -w' -o favget ./cmd/server

# Run stage
FROM gcr.io/distroless/static-debian12

ENV PORT=8080
WORKDIR /app

COPY --from=build --chown=nonroot:nonroot /app/favget /app/favget

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/favget"]
