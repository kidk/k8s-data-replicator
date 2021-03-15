FROM golang:1.15-alpine3.13 AS builder

COPY ${PWD} /app
WORKDIR /app

# Toggle CGO based on your app requirement. CGO_ENABLED=1 for enabling CGO
RUN CGO_ENABLED=0 go build -ldflags '-s -w -extldflags "-static"' -o /app/replicator ./src/*.go
# Use below if using vendor
# RUN CGO_ENABLED=0 go build -mod=vendor -ldflags '-s -w -extldflags "-static"' -o /app/appbin *.go

FROM alpine:3.13
LABEL MAINTAINER Samuel Vandamme <svandamme@newrelic.com>

# Following commands are for installing CA certs (for proper functioning of HTTPS and other TLS)
RUN apk --update add ca-certificates && \
    rm -rf /var/cache/apk/*

# Add new user 'appuser'
RUN adduser -D appuser
USER appuser

COPY --from=builder /app /home/appuser/app

WORKDIR /home/appuser/app

CMD ["./replicator"]
