FROM --platform=$BUILDPLATFORM golang:alpine AS builder

LABEL stage=gobuilder
LABEL org.opencontainers.image.source=https://github.com/im-kulikov/resolvex

ENV CGO_ENABLED=0

RUN apk update --no-cache && apk add --no-cache tzdata upx

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o /app/resolvex /build/cmd/resolvex \
    && upx -9 /app/resolvex


FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo/Europe/Moscow /usr/share/zoneinfo/Europe/Moscow
COPY --from=builder /app/resolvex /app/resolvex

CMD ["/app/resolvex"]
