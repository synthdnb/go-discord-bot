FROM golang:1.21.3-alpine3.18 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -v -o /usr/local/bin/app ./...

FROM alpine:3.18
WORKDIR /usr/src/app
COPY --from=builder /usr/local/bin/app /usr/local/bin/app
CMD ["app"]
