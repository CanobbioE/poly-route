# Poly-Route
A pluggable protocol-agnostic<sup>*</sup> transparent reverse proxy

![img.png](img.png)

Poly-Route provides a single global entrypoint for regional APIs. The region is resolved automatically using data from another service instead of being chosen by the caller.

*Work in progress. Currently, supports gRPC and HTTP.*

## Table of Contents
- [What's in the box](#whats-in-the-box)
- [Run the demo](#run-the-demo)
    - [In Docker](#in-docker)
    - [From CLI locally](#from-cli-locally)
        - [Start the test servers](#start-the-test-servers)
        - [Start the proxy with the test config](#start-the-proxy-with-the-test-config)
        - [Start the test client](#start-the-test-client)
- [Next steps](#next-steps)
    - [More testing](#more-testing)
    - [Setup](#setup)
- [Roadmap](#roadmap)
- [Configuration Guide](#configuration-guide)
    - [HTTP](#http)
    - [gRPC](#grpc)
    - [Region Retriever](#region-retriever)
    - [Flow](#flow)

## What's in the box
The proxy itself, fully configurable and pluggable (the bright pink box in the above image).

You must provide your own region resolver and your own client and backend services

**Notes**
* A database IS NOT required for the region resolver if you can resolve without it
* You DO NOT need to define a client per protocol
* Services MUST be reachable from the proxy
* The region resolver MUST BE reachable from the proxy

## Run the demo

### In Docker
Using the utility makefile:
```
make test
```

or manually, by starting the example environment with docker compose

```shell
docker compose up
```

This starts three services:
- `poly-route`: the proxy server
- `test-servers`: mocked backends
    - two HTTP servers (EU and US)
    - two gRPC servers (EU and US)
    - one HTTP server that returns region values
- `test-client`: a client that will call all the backends through the proxy

### From CLI locally

#### Start the test servers
```shell
go run ./example/server
```

The following ports must be available:
- `1234`
- `8085`
- `8081`
- `9095`
- `9091`

#### Start the proxy with the test config
```shell
CONFIG_FILE_PATH=./example/config.yaml go run .
```

Ports `9999` and `8888` must be available

#### Start the test client
```shell
go run ./example/client
```

Example output:
```shell
Received BiDirectionalStream: Stream response for res-123
Received BiDirectionalStream: Stream response for res-456
Received BiDirectionalStream: Stream response for res-789
Received ClientStream: res-123;res-456;res-789;
Received ServerStream: res-123 response #0
Received ServerStream: res-123 response #1
Received ServerStream: res-123 response #2
Received ServerStream: res-123 response #3
Received ServerStream: res-123 response #4
Received BiDirectionalStream: Stream response for res-123
Received BiDirectionalStream: Stream response for res-456
Received BiDirectionalStream: Stream response for res-789
Received ClientStream: res-123;res-456;res-789;
Received ServerStream: res-123 response #0
Received ServerStream: res-123 response #1
Received ServerStream: res-123 response #2
Received ServerStream: res-123 response #3
Received ServerStream: res-123 response #4
data:"Response for res-123"
data:"Response for res-123"
{"backend": eu, "addr": localhost:8085, "path": GET/}
{"backend": eu, "addr": localhost:8085, "path": GET/}
{"backend": us, "addr": localhost:8081, "path": GET/}
{"backend": us, "addr": localhost:8081, "path": GET/}
```

## Next steps

### More testing
Experiment with [config.yaml](example/config.yaml) or [docker-config.yaml](example/docker-config.yaml). Update [test client](example/client/main.go) and [test servers](example/server/main.go) if needed

You can also bypass the client and call the proxy directly
- use [curl](https://curl.se/) for HTTP on `http://localhost:8888`
- use [grpcurl](https://github.com/fullstorydev/grpcurl) for gRPC on `localhost:9999`

Protobuf definitions are in [example/mock/proto](example/mock/proto). The HTTP server accepts any method on `/`

**Important**
Set the correct region value in each request
- for gRPC set metadata key `poly-route-region`
- for HTTP set header `X-Poly-Route-Region`

Supported values are listed under `region_resolver.mapping` in [config.yaml](example/config.yaml)
```
europe-west1
eu-west1
us-east1
```

### Setup
Poly-Route is plug and play
- create a region retrieval endpoint if you need one
- define config.yaml
- run the provided docker image in your stack



## Roadmap
- support nested keys in region resolver responses
- support for GraphQL
- logger configuration
- support POST for region resolver
- improve logs
- improve error messages
- support more protocols


## Configuration Guide

### HTTP

```yaml
http:
  listen: "8888"
  destinations:
    /:
      euw1: "http://localhost:8085"
      use1: "http://localhost:8081"
```

Proxy listens on port 8888. All HTTP requests to `/` are routed by region:
- `euw1` -> `http://localhost:8085`
- `use1` -> `http://localhost:8081`

### gRPC

```yaml
grpc:
  listen: "9999"
  destinations:
    /mockserver.v1.MockService/Invoke":
      euw1: "localhost:9095"
      use1: "localhost:9091"
```

Proxy listens on port 9999. Each RPC method is routed to the correct backend by region.

### Region Retriever

```yaml
region_retriever:
  type: "http"
  url: "http://localhost:1234/userinfo"
  method: "GET"
  query_param: "user_id"
  region_resolver:
    type: "map"
    field: "country"
    mapping:
      europe-west1: "euw1"
      eu-west1: "euw1"
      us-east1: "use1"
```

Proxy queries `http://localhost:1234/userinfo?user_id=...`.  
The `country` field in the response is mapped to an internal region key.
- `europe-west1` or `eu-west1` -> `euw1`
- `us-east1` -> `use1`

### Flow

1. Client sends HTTP or gRPC request to proxy
2. Proxy calls region retriever to resolve region
3. Resolver maps `country` to `euw1` or `use1`
4. Proxy forwards request to backend defined under that region

