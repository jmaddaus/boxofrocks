FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /bin/bor ./cmd/bor

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/bor /usr/local/bin/bor

# Data directory — DefaultConfig() resolves to $HOME/.boxofrocks.
# Declare as a volume so data persists across container restarts
# even without an explicit -v mount (Docker manages the volume).
# Override by mounting a different path:
#   docker run -v /my/data:/home/bor/.boxofrocks ...
ENV HOME=/home/bor
VOLUME /home/bor/.boxofrocks

EXPOSE 8042

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8042/health || exit 1

ENTRYPOINT ["bor"]
CMD ["daemon", "start", "--foreground"]
