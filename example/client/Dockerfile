FROM golang:1.24.5-bookworm AS build-env
LABEL authors="edoardo"

WORKDIR /go/src/github.com/CanobbioE/poly-route
COPY . .
WORKDIR /go/src/github.com/CanobbioE/poly-route/example/client


RUN CGO_ENABLED=0 go build -o example-client -ldflags "-w -s"

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=build-env --chown=nonroot /go/src/github.com/CanobbioE/poly-route/example/client/example-client /app/example-client
COPY --from=build-env --chown=nonroot /go/src/github.com/CanobbioE/poly-route/example/config.yaml /app/config.yaml

USER 65532
ENTRYPOINT ["/app/example-client"]
