FROM golang:1.26.0-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/core ./cmd/

FROM alpine:3.21
WORKDIR /app
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /out/core /app/bin/core
RUN chmod +x /app/bin/core
EXPOSE 6767
CMD ["./bin/core"]