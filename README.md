# poly-route
A pluggable protocol-agnostic transparent reverse proxy.
![img.png](img.png)

This project stems from the need of having a single, global, entrypoint for regional APIs.
Our constraint was that the region must be inferred by a value provided by another service and must not be picked by the caller.

# Run the demo
## In Dokcer
Using `docker compose` it's possible to run the provided [example](example/):
```shell
docker compose up
```

This will start up three services:
- `poly-route`: the proxy server
- `test-servers`: mocked backend servers:
  - 2 HTTP backend servers (EU and US regions)
  - 2 gRPC backend servers (EU and US regions)
  - 1 HTTP server to retrieve the region value
- `test-client`: a simple client that will try to reach all 4 backend clients

## From CLI locally
### Start the test servers
```shell
go run ./example/server
```

Make sure the following ports are available:
- `1234`
- `8085`
- `8081`
- `9095`
- `9091`

### Start the proxy with the test config
```shell
CONFIG_FILE_PATH=./example/config.yaml go run .
```

Make sure port `9999` and port `8888` are available.

### Start the test client
```shell
go run ./example/client
```

The output should look something like:
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


# Next steps

## More testing!
Feel free to play around with the test [config.yaml](example/config.yaml) (or [docker-config,yaml](example/docker-config.yaml)), just remember to update the [test client](example/client/main.go) and the [test servers](example/server/main.go)

You could also try to reach the backend services without the test client.
Simply send a [cURL](https://curl.se/) or [grpccurl](https://github.com/fullstorydev/grpcurl) request to the appropriate proxy url:
- `localhost:9999` for gRPC requests
- `http://localhost:8888` for HTTP requests

You can find the protobuff messages definition inside the folder [example/mock/proto](example/mock/proto).
The HTTP server supports any method on the `/` endpoint.

**IMPORTANT**:
Make sure to set the appropriate region value in the requests:
- for gRPC, set the metadata value `poly-route-region`
- for HTTP, set the header `X-Poly-Route-Region`

The supported values are listed in [config.yaml](example/config.yaml) under the region_resolver's mapping:
```
    europe-west1
    eu-west1
    us-east1
```

## Setup
poly-route is a plug-and-play service:
- create a region retrieval endpoint if you don't have one already
- define your config.yaml
- use the provided docker image in your architecture
