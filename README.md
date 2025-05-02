# Google Maps API Cache

A high-performance Google Maps API caching server that reduces the number of queries made to the Google Maps API. The proxy caches duplicate requests using Redis and is fully compliant with Google Maps API guidelines. It also adds a privacy layer for client-side API calls.

## Features

- Redis-based caching for high performance and reliability
- Configurable cache timeout (up to 30 days as per Google's guidelines)
- Support for custom base URLs
- GCP-compatible logging
- CORS support
- Health check endpoint
- Docker and docker-compose support
- Multi-server support with Redis DB selection and key prefixing

## Installation with docker-compose

1. Create a `.env` file with your desired configuration (see Environment Variables section below)
2. Run `docker-compose up -d` to start the server and Redis
3. Make your Google Maps API queries to `http://localhost/`

## Environment Variables

- `REDIS_HOST`: Redis server hostname (default: "redis")
- `REDIS_PORT`: Redis server port (default: "6379")
- `REDIS_DB`: Redis database number to use (default: 0)
- `REDIS_PREFIX`: Prefix for cache keys, useful for multi-server setups (default: "")
- `SERVER_PORT`: Port for the geocache server (default: "80")
- `BASE_URL`: Base URL for Google Maps API (default: "https://maps.googleapis.com")
- `CACHE_TIMEOUT_HOURS`: Cache entry lifetime in hours (default: 720 hours/30 days)
- `LOG_FORMAT`: Logging format, set to "gcp" for Google Cloud Platform format (default: standard logging)
- `INFLUX_DSN`: InfluxDB connection string (DSN). Example: `http://localhost:8086?org=my-org&bucket=my-bucket&token=my-token`. If set (and sample rate > 0), cache hit/miss events will be recorded to InfluxDB.
- `INFLUX_SAMPLE_RATE`: Float between 0 and 1. Probability of recording a cache event to InfluxDB (e.g., `0.1` for 10% sampling, `1.0` for all events, `0` disables recording).

## InfluxDB Integration

If you want to monitor cache hits and misses in InfluxDB, set the following environment variables:

- `INFLUX_DSN`: The connection string for your InfluxDB instance. Example:
  ```
  http://localhost:8086?org=my-org&bucket=my-bucket&token=my-token
  ```
  - `org`: InfluxDB organization (required, but can be `ignored` for some setups)
  - `bucket`: InfluxDB bucket (database/table)
  - `token`: InfluxDB API token
- `INFLUX_SAMPLE_RATE`: Sampling rate (float between 0 and 1). Example: `0.25` records 25% of events.

When enabled, the server will record a `cache_event` measurement in InfluxDB for each cache hit or miss (according to the sample rate). Each event includes:

- `event` (tag): `hit` or `miss`
- `api` (field): the base API path (e.g., `/maps/api/geocode/json`)
- `api_key` (field): obfuscated API key (first 4 and last 4 characters, or full key if ≤8 chars)
- `cache_key` (field): the cache key (hash)

If the API key is missing, the event is not recorded. InfluxDB errors are logged as warnings but do not affect server operation.

## Multi-Server Configuration

You can run multiple instances of the server using the same Redis instance by configuring different database numbers or key prefixes:

### Using Different Redis Databases
```bash
# Production server using Redis DB 1
REDIS_DB=1 ./server

# Staging server using Redis DB 2
REDIS_DB=2 ./server
```

### Using Key Prefixes
```bash
# Production server with 'prod' prefix
REDIS_PREFIX=prod ./server

# Staging server with 'staging' prefix
REDIS_PREFIX=staging ./server
```

You can also combine both approaches:
```bash
# Production server on DB 1 with 'prod' prefix
REDIS_DB=1 REDIS_PREFIX=prod ./server

# Staging server on DB 2 with 'staging' prefix
REDIS_DB=2 REDIS_PREFIX=staging ./server
```

## API Usage

You can pass your Google Maps API key in one of two ways:
1. Include it in the request URL as a query parameter
2. Pass it in the `X-Maps-API-Key` header

The server will cache responses based on the request path and parameters. Subsequent identical requests will be served from the cache until the cache timeout is reached.

### Response Headers

- `X-Cache`: Indicates if the response was served from cache ("HIT") or from the Google Maps API ("MISS")
- Standard CORS headers are included for browser compatibility

## Development

The project is written in Go 1.21+ and uses:
- [go-redis/v9](https://github.com/redis/go-redis) for Redis integration
- Standard Go HTTP server for handling requests

## Docker Configuration

The included `docker-compose.yml` sets up both the geocache server and Redis. The Redis data is persisted using a named volume.

To modify the server port, update the ports mapping in `docker-compose.yml`:
```yaml
ports:
  - "your-port:80"
```

## Troubleshooting

### Redis Connection Issues

If you see Redis connection errors:
1. Check that Redis is running: `docker-compose ps`
2. Verify Redis host and port in your environment variables
3. Check Redis logs: `docker-compose logs redis`
4. Verify the Redis DB number is valid and accessible
5. Check that the Redis prefix (if used) follows Redis key naming conventions

### Cache Not Working

1. Verify Redis is running and accessible
2. Check the request URL matches exactly (including query parameters)
3. Verify cache timeout setting isn't set too low
4. If using prefixes, check that the prefix is being applied correctly
5. Verify you're connecting to the correct Redis database number

## Legal Considerations

Google allows caching of results for up to 30 days according to their terms (https://developers.google.com/maps/premium/optimize-web-services). You are responsible for ensuring your usage complies with Google's terms of service.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

## License

This project is open source and available under the MIT License.
