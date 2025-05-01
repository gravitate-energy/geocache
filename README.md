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

## Installation with docker-compose

1. Create a `.env` file with your desired configuration (see Environment Variables section below)
2. Run `docker-compose up -d` to start the server and Redis
3. Make your Google Maps API queries to `http://localhost/`

## Environment Variables

- `REDIS_HOST`: Redis server hostname (default: "redis")
- `REDIS_PORT`: Redis server port (default: "6379")
- `SERVER_PORT`: Port for the geocache server (default: "80")
- `BASE_URL`: Base URL for Google Maps API (default: "https://maps.googleapis.com")
- `CACHE_TIMEOUT_HOURS`: Cache entry lifetime in hours (default: 720 hours/30 days)
- `LOG_FORMAT`: Logging format, set to "gcp" for Google Cloud Platform format (default: standard logging)

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

### Cache Not Working

1. Verify Redis is running and accessible
2. Check the request URL matches exactly (including query parameters)
3. Verify cache timeout setting isn't set too low

## Legal Considerations

Google allows caching of results for up to 30 days according to their terms (https://developers.google.com/maps/premium/optimize-web-services). You are responsible for ensuring your usage complies with Google's terms of service.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

## License

This project is open source and available under the MIT License.
