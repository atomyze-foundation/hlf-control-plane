# hlf-control-plane

Service for hlf fabric administration

## TOC

- [hlf-control-plane](#-hlf-control-plane)
  - [TOC](#-toc)
  - [Description](#-description)
  - [Open API](#-open-api)
  - [Configuration](#-configuration)
  - [Monitoring](#-monitoring)
  - [Development](#-development)
    - [Utils](#-utils)
    - [Protobuf](#-protobuf)
    - [Code generation](#-code-generation)
  - [License](#-license)
  - [Links](#-links)

## Description

A utility service that provides simplified access to fabric administrative functions, such as creating channels, installing chaincodes, and updating channels.

## Open API

The service provides OpenAPI (Swagger) functionality. The list of methods and request formats is available at the [link](proto/plane.swagger.json).

## Configuration

```yaml
# Msp ID of your organization
mspId: atomyzeMSP
# Logging level
logLevel: debug
# Access token value
accessToken: "my_awesome_token"
# Path to the certificate and private key of your identity
identity:
  cert: certs/atomyze_admin.pem
  key: certs/atomyze_admin_key.pem
# or bccsp config of pkcs11
#  bccsp:
#    Default: PKCS11
#    PKCS11:
#      immutable: false
#      label: Org_Admin
#      library: /usr/lib/softhsm/libsofthsm2.so
#      pin: 123321
#      hash: SHA2
#      security: 256

# Tls credentials for mutual tls connection
tls:
  cert: cert.pem
  key: key.pem
  ca: ca.pem

# List of peers managed by organization
peers:
  - host: peer1.atomyze.io:7051
  - host: peer2.atomyze.io:7051

# ports configuration for grpc and http
listen:
  http: ":8080"
  grpc: ":8081"
```

## Monitoring

The service includes a built-in **healthcheck** mechanism, available without authentication at the path `/v1/healthz`, returning information in the following format:

```json
{
  "status": "SERVING"
}
```

## Development

### Utils

For the generation of tools needed for work, use

```bash
make build-tools
```

Tools and their dependencies are described in the `tools` module. The installation path is `$(pwd)/bin` by default.

### Protobuf

1. protodep - protobuf dependency management
2. buf - generate and lint protobuf code

To download vendored protobuf dependencies:

```bash
bin/protodep up
```

### Code generation

Use `go generate` in the root path to generate code from `.proto` and `swagger` client for tests:

```bash
go generate
```

## License

[Default license](LICENSE)

## Links

- [protodep](https://github.com/stormcat24/protodep)
- [buf](https://buf.build)
