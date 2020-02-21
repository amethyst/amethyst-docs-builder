FROM golang:alpine as build-server

RUN apk update && apk add --no-cache git

RUN mkdir -p /app
WORKDIR /app
ADD go.mod go.sum webhook-server.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o server

FROM amethyst-docker.jfrog.io/amethyst-builder

RUN mkdir -p /app
WORKDIR /app
COPY --from=build-server /app/server .
RUN chmod ugo+x server
RUN rustup update stable

ADD run.sh CHECKS Procfile 404.html ./