FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /bin/bor ./cmd/bor

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/bor /usr/local/bin/bor

EXPOSE 8042

ENTRYPOINT ["bor"]
CMD ["daemon", "start"]
