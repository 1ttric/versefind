FROM golang:alpine AS build
RUN apk update && \
    apk add --no-cache git ca-certificates && \
    update-ca-certificates

WORKDIR /app/src
COPY . .
RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -v -trimpath -o /app/build

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/build /app/build

ENTRYPOINT ["/app/build"]