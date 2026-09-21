FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd

FROM alpine:3.20
# LoadLocation(APP_TIMEZONE) reads the zoneinfo DB; alpine ships none by default.
RUN apk add --no-cache tzdata ca-certificates wget
COPY --from=builder /out/server /server
EXPOSE 14000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:14000/health || exit 1
ENTRYPOINT ["/server"]
