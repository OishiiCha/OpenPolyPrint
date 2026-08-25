# Frontend build stage
FROM node:22-bookworm AS frontend

WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Backend build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /openpolyprint ./cmd/openpolyprint

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /openpolyprint /usr/local/bin/openpolyprint
COPY --from=frontend /app/dist ./frontend/dist

RUN mkdir -p /data

EXPOSE 80 443

ENTRYPOINT ["openpolyprint"]
CMD ["-addr", ":443"]
