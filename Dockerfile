# Build stage
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o favget ./cmd/server

# Run stage
FROM gcr.io/distroless/base-debian12
ENV PORT=8080
WORKDIR /app
COPY --from=build /app/favget /app/favget
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/favget"]
