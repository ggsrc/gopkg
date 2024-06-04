# rpcutil

rpcutil is a utility package for grpc server.

use environment variable to control the server parameter

### grpc

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| GRPC_PORT       | The port number on which the gRPC server is running. | 9090               |
| GRPC_RAVEN_DSN | The DSN for the Sentry client.	                     | "" (empty string)  |
| GRPC_VERBOSE | Enable verbose logging.	                             | false              |


### redis

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| REDIS_HOST       |The hostname or IP address of the Redis server.	 | 127.0.0.1               |
| REDIS_PORT | The port number on which the Redis server is running.     | 6379          |
| REDIS_PASSWORD | The password used to authenticate with the Redis server. | "" (empty string) |
| REDIS_IS_FAILOVER | Indicates if failover support is enabled.	             | false             |
| REDIS_IS_ELASTICACHE | Indicates if the Redis instance is an ElastiCache instance. | false |
| REDIS_IS_CLUSTER_MODE | Indicates if Redis is running in cluster mode.	         | false             |
| REDIS_CLUSTER_ADDRS | Addresses of the nodes in the Redis cluster.	         | "" (empty slice)  |
| REDIS_CLUSTER_MAX_REDIRECTS | Maximum number of redirects to follow in cluster mode. | 3                 |
| REDIS_READ_TIMEOUT | The duration to wait before timing out on read operations. | 3s               |
| REDIS_POOL_SIZE | The maximum number of connections in the pool.	         | 50                |


### dcache

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| DCACHE_READINTERVAL       | The interval to read the cache data.	 | 500ms               |
| DCACHE_ENABLESTATS | Enable the cache statistics.     | true          |
| DCACHE_ENABLETRACE | Enable the cache tracing. | true |
| DCACHE_INMEMCACHESIZE | The size of the in-memory cache. | 52428800 (50MB) |


### postgres

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| POSTGRES_PORT       | Port number for PostgreSQL                           | 5432               |
| POSTGRES_HOST | Host address for PostgreSQL                          | localhost          |
| POSTGRES_USERNAME | Username for PostgreSQL                              | postgres           |
| POSTGRES_PASSWORD | Password for PostgreSQL	                             | my-secret          |
| POSTGRES_DBNAME | Database name for PostgreSQL                         | wpgx_test_db       |
| POSTGRES_MAXCONNS | Maximum number of idle connections                   | 100                |
| POSTGRES_MINCONNS | Minimum number of idle connections	                  | 0                  |
| POSTGRES_MAXCONNLIFETIME | Maximum lifetime of connections	                     | 6h                 |
| POSTGRES_MAXCONNIDLETIME | Maximum idle time of connections                     | 1m                 |
| POSTGRES_ENABLEPROMETHEUS | Enable Prometheus metrics for PostgreSQL connections | true               |
| POSTGRES_ENABLETRACING | Enable tracing for PostgreSQL connections	           | true               |
| POSTGRES_APPNAME | Application name for PostgreSQL	                     | (must be provided) |


### healthcheck:

health check is a http server that expose the health status of the grpc server

| environment var        | Description                     | default value |
|------------------------|---------------------------------|---------------|
| HEALTHCHECK_PORT       | health check expose port        | 8080          |
| HEALTHCHECK_READYCOUNT | Number of successful ready checks required before marking the service as ready | 0             |
| HEALTHCHECK_LIVECOUNT | Number of successful liveness checks required before marking the service as alive | 3             |
| HEALTHCHECK_PROBEINTERVAL | Interval between health check probes | 5s            |
| HEALTHCHECK_PROBETIMEOUT | Timeout for each health check probe	| 5s            |
| HEALTHCHECK_READY | Whether Initial ready checker   | true          |
| HEALTHCHECK_ALIVE | Whether Initial alive checker   | true          |
