<p align="center">
  <img src="https://raw.githubusercontent.com/PKief/vscode-material-icon-theme/ec559a9f6bfd399b82bb44393651661b08aaf7ba/icons/folder-markdown-open.svg" width="100" alt="project-logo">
</p>
<p align="center">
    <h1 align="center">gopkg</h1>
</p>
<p align="center">
    <em>Efficiency Unleashed.</em>
</p>
<p align="center">
	<!-- local repository, no metadata badges. -->
<p>
<p align="center">
		<em>Developed with the software and tools below.</em>
</p>
<p align="center">
	<img src="https://img.shields.io/badge/YAML-CB171E.svg?style=default&logo=YAML&logoColor=white" alt="YAML">
	<img src="https://img.shields.io/badge/GitHub%20Actions-2088FF.svg?style=default&logo=GitHub-Actions&logoColor=white" alt="GitHub%20Actions">
	<img src="https://img.shields.io/badge/Go-00ADD8.svg?style=default&logo=Go&logoColor=white" alt="Go">
</p>

<hr>

##  Overview
[![codecov](https://codecov.io/gh/ggsrc/gopkg/branch/main/graph/badge.svg?token=LUJBQBEET1)](https://codecov.io/gh/ggsrc/gopkg)


---

##  Repository Structure

```sh
└── ./
    ├── .github
    │   ├── dependabot.yml
    │   └── workflows
    ├── Makefile
    ├── README.md
    ├── database
    │   ├── cache
    │   └── wpgx
    ├── env
    │   ├── env.go
    │   ├── go.mod
    │   ├── host.go
    │   └── service.go
    ├── go.work
    ├── go.work.sum
    ├── goodns
    │   ├── go.mod
    │   ├── go.sum
    │   └── goodns.go
    ├── grpc
    │   ├── client.go
    │   ├── go.mod
    │   ├── go.sum
    │   ├── server.go
    │   └── util.go
    ├── health
    │   ├── go.mod
    │   ├── go.sum
    │   └── health.go
    ├── interceptor
    │   ├── go.mod
    │   ├── go.sum
    │   ├── grpc
    │   ├── http
    │   └── metadata
    ├── metric
    │   ├── go.mod
    │   ├── go.sum
    │   └── metric.go
    ├── rpcutil
    │   ├── Init.go
    │   ├── README.md
    │   ├── go.mod
    │   ├── go.sum
    │   └── resource.go
    ├── utils
    │   ├── go.mod
    │   ├── go.sum
    │   └── trace.go
    └── zerolog
        ├── go.mod
        ├── go.sum
        ├── init.go
        ├── log
        └── otel.go
```

---

##  Modules

<details closed><summary>grpc</summary>

| File                        | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ---                         | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| [server.go](grpc/server.go) | Establishes a gRPC server, configuring it with interceptors for logging, metrics, and context management, and integrates OpenTelemetry for distributed tracing. Facilitates the registration of services, manages server lifecycle, and ensures graceful shutdown.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| [client.go](grpc/client.go) | Establishes a gRPC client connection, integrating logging, metrics, and error tracking through interceptors, enhancing observability and reliability in client-server communication.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| [util.go](grpc/util.go)     | Determines device type from user agent strings, extracts IP addresses from HTTP requests, and manages metadata retrieval for context-aware operations in gRPC services.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

</details>

<details closed><summary>health</summary>

| File                          | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---                           | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| [health.go](health/health.go) | Manages health checks for services, ensuring readiness and liveness by periodically probing dependencies and serving HTTP endpoints to report status, enhancing system reliability and uptime.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |

</details>

<details closed><summary>goodns</summary>

| File                          | Summary                                                                                                                                                                                                                  |
| ---                           | ---                                                                                                                                                                                                                      |
| [goodns.go](goodns/goodns.go) | Implements DNS A record lookups, offering flexibility in querying either default or specified DNS servers over TCP or UDP, enhancing domain resolution capabilities within the repositorys networking utilities.         |

</details>

<details closed><summary>utils</summary>

| File                       | Summary                                                                                                                                                                                                                    |
| ---                        | ---                                                                                                                                                                                                                        |
| [trace.go](utils/trace.go) | Initiates and configures tracing for the application, integrating OpenTelemetry to capture service-level and internal span details, enhancing observability and debugging capabilities.                                    |

</details>

<details closed><summary>env</summary>

| File                         | Summary                                                                                                                                                                     |
| ---                          | ---                                                                                                                                                                         |
| [env.go](env/env.go)         | Determines the environment mode by checking the ENV or OTEL_DEPLOYMENT_ENVIRONMENT environment variables, classifying it as production, beta, staging, local, or unit test. |
| [service.go](env/service.go) | Initializes and manages the service name by retrieving it from environment variables or setting it manually, ensuring a consistent identifier across the application.       |
| [host.go](env/host.go)       | Initializes and retrieves the host name from environment variables, defaulting to UnknownHost if not set, ensuring consistent identification across the system.             |

</details>

<details closed><summary>zerolog</summary>

| File                       | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---                        | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| [otel.go](zerolog/otel.go) | Initializes a default logging system integrating OpenTelemetry for distributed tracing and zerolog for structured logging, adapting batch size based on environment staging status.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| [init.go](zerolog/init.go) | Initializes the logging framework, adjusting log level based on debug mode and integrating OpenTelemetry for enhanced observability.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |

</details>


<details closed><summary>rpcutil</summary>

| File                               | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---                                | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| [resource.go](rpcutil/resource.go) | Manages application resources, orchestrating their startup and graceful shutdown, while monitoring for system signals and errors to ensure stability and reliability.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| [Init.go](rpcutil/Init.go)         | Configures and initializes various components of the application, including gRPC server, database, cache, health checks, and metrics, with customizable options for debugging and application naming.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

</details>

<details closed><summary>metric</summary>

| File                          | Summary                                                                                                                                                                                                                                                |
| ---                           | ---                                                                                                                                                                                                                                                    |
| [metric.go](metric/metric.go) | Implements a metrics server that exposes Prometheus metrics via HTTP, configurable through environment variables and listening on a specified port.                                                                                                    |

</details>

<details closed><summary>database.cache</summary>

| File                                          | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ---                                           | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| [config.go](database/cache/config.go)         | Configures and initializes a Redis client with various modes and failover options, integrating tracing and metrics instrumentation for monitoring and reliability.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| [dcache.go](database/cache/dcache.go)         | Establishes a distributed caching system, integrating Redis and in-memory caching for efficient data retrieval and storage, configurable via environment variables to optimize performance and monitoring.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| [cache.go](database/cache/cache.go)           | Implements a caching mechanism using Redis, providing methods to retrieve, set, and invalidate cache entries with optional expiration and locking for concurrent access control.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| [init.go](database/cache/init.go)             | Initializes the caching system by establishing a Redis connection and initializing a distributed cache instance, ensuring efficient data storage and retrieval for the application.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |

</details>

<details closed><summary>database.wpgx</summary>

| File                             | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ---                              | ---                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| [pool.go](database/wpgx/pool.go) | Initializes a WPGX database pool, integrating environment configuration and logging for robust database connectivity within the applications architecture.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| [init.go](database/wpgx/init.go) | Initializes the database connection pool, ensuring a timed context for establishing a PostgreSQL connection, critical for managing database interactions within the application.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |

</details>

<details closed><summary>zerolog.log</summary>

| File                         | Summary                                                                                                                                                                                  |
| ---                          | ---                                                                                                                                                                                      |
| [ctx.go](zerolog/log/ctx.go) | Enhances logging capabilities by integrating context-aware logging, configurable output, and customizable logging levels, ensuring detailed and flexible logging across the application. |

</details>


<details closed><summary>interceptor.grpc</summary>

| File                                        | Summary                                                                                                                                                                                                                        |
| ---                                         | ---                                                                                                                                                                                                                            |
| [recovery.go](interceptor/grpc/recovery.go) | Implements error recovery for gRPC services, capturing panics and logging detailed stack traces, enhancing system stability and aiding in debugging.                                                                           |
| [context.go](interceptor/grpc/context.go)   | Enhances gRPC communication by intercepting and enriching context with metadata, including request source, JWT tokens, access tokens, Galxe ID, and origin, ensuring secure and informed processing of requests and responses. |

</details>

<details closed><summary>interceptor.http</summary>

| File                                  | Summary                                                                                                                                                                    |
| ---                                   | ---                                                                                                                                                                        |
| [proxy.go](interceptor/http/proxy.go) | Enhances HTTP request handling by adjusting the `RemoteAddr` field based on the `X-Forwarded-For` header, ensuring accurate client IP tracking in reverse proxy scenarios. |

</details>

<details closed><summary>interceptor.metadata</summary>

| File                                            | Summary                                                                                                                                                                           |
| ---                                             | ---                                                                                                                                                                               |
| [metadata.go](interceptor/metadata/metadata.go) | Defines context keys and request sources for metadata storage, facilitating identification and categorization of incoming requests within the applications interceptor framework. |

</details>


---