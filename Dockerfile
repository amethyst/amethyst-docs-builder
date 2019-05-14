FROM golang:alpine as build-server

RUN apk update && apk add --no-cache git

RUN mkdir -p /app
WORKDIR /app
ADD go.mod .
ADD go.sum .
ADD webhook-server.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o server

FROM rust:latest
RUN apt-get update && apt-get install git

RUN mkdir -p /app
WORKDIR /app
COPY --from=build-server /app/server .
RUN chmod ugo+x server

ADD *.sh ./
ADD CHECKS .
ADD Procfile .

RUN cargo install cargo-update; \
    cargo install mdbook