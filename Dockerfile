# syntax=docker/dockerfile:1
FROM golang:1.16-alpine

WORKDIR /app
COPY . ./

RUN go build -o /example

RUN chmod +x /example
CMD ["/example"]

