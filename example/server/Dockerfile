FROM golang:1.24.5-bookworm AS build-env
LABEL authors="edoardo"

WORKDIR /go/src/github.com/CanobbioE/poly-route
COPY . .
WORKDIR /go/src/github.com/CanobbioE/poly-route/example/server

RUN CGO_ENABLED=0 go build -o example-server -ldflags "-w -s"

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=build-env --chown=nonroot /go/src/github.com/CanobbioE/poly-route/example/server/example-server /app/example-server

USER 65532
ENTRYPOINT ["/app/example-server"]
