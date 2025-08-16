FROM golang:1.24.5-bookworm AS build-env
LABEL authors="edoardo"

WORKDIR /go/src/github.com/CanobbioE/poly-route
COPY . .


ENV GO111MODULE=on
ENV GOFIPS140=latest
RUN CGO_ENABLED=0 GODEBUG=fips140=only go build -o poly-route -ldflags "-w -s"

FROM gcr.io/distroless/base-debian12:nonroot

ENV GOFIPS140=latest

COPY --from=build-env --chown=nonroot /go/src/github.com/CanobbioE/poly-route/poly-route /app/poly-route

USER 65532
ENTRYPOINT ["/app/poly-route"]
