# Google Maps Proxy Load Tester

This tool lets you load test your geocaching server that proxies requests to Google Maps API.

## Prerequisites

- Python 3.7+
- pip (Python package manager)

## Installation

1. Clone this repository or download the files
2. Install the required dependencies:

```bash
pip install locust
```

## Configuration

The script requires a Google Maps API key which should be set as an environment variable:

### Linux/macOS:
```bash
export MAPS_API_KEY="your_google_maps_api_key_here"
```

## Usage

### Web UI Mode

1. Start Locust with your server URL:
```bash
# For development environment
locust -H https://geocache-dev.gravitate.energy

# For production environment
locust -H https://geocache.gravitate.energy
```

2. Open http://localhost:8089 in your browser
3. Set the desired number of users and spawn rate
4. Start the test and monitor results in real-time

### Headless Mode (Command Line)

To run without the web interface (good for automation):

```bash
# For development environment
locust -f locustfile.py --headless -H https://geocache-dev.gravitate.energy -u 50 -r 10 -t 5m

# For production environment
locust -f locustfile.py --headless -H https://geocache.gravitate.energy -u 50 -r 10 -t 5m
```

Parameters:
- `-u 50`: Simulate 50 users
- `-r 10`: Spawn 10 users per second
- `-t 5m`: Run test for 5 minutes

## Customization

You can modify `locustfile.py` to:
- Change the sample locations
- Add more API endpoints to test
- Adjust request frequency and patterns
- Configure failure thresholds

## Monitoring 502 Errors

The script automatically tracks and reports 502 errors. Check the console output or the web UI for:
- Total requests
- Failure rate
- Number of 502 errors

## Stopping the Test

- In Web UI mode: Click the "Stop" button
- In Headless mode: The test will stop after the specified duration, or press Ctrl+C

## Cache Hit/Miss Test Scripts

This directory contains scripts to emulate Google Maps API requests (directions and distance matrix) against a locally running geocache server. These scripts help verify cache hits and misses by repeating requests and observing the `X-Cache` response header.

### Usage

1. Install dependencies:
   ```sh
   pip install -r requirements.txt
   ```
2. Run the scripts:
   ```sh
   python test_directions.py
   python test_distance_matrix.py
   ```

Adjust the server URL and API key in the scripts as needed.