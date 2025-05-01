from typing import Dict, Any, List 
from locust import HttpUser, task, between, events
import random
import time
import os

# Sample origins and destinations for directions API
SAMPLE_LOCATIONS: List[Dict[str, str]] = [
    {"origin": "Chicago,IL", "destination": "St Louis,MO"},
    {"origin": "New York,NY", "destination": "Boston,MA"},
    {"origin": "Seattle,WA", "destination": "Portland,OR"},
    {"origin": "Miami,FL", "destination": "Orlando,FL"},
    {"origin": "Austin,TX", "destination": "Houston,TX"}
]

# Get API key from environment variable
API_KEY = os.environ.get("MAPS_API_KEY")

# Check if API key is available
if not API_KEY:
    raise ValueError("MAPS_API_KEY environment variable is not set. Please export MAPS_API_KEY='your_key_here'")

class GeoServerUser(HttpUser):
    wait_time = between(0.5, 2)
    
    def on_start(self) -> None:
        self.start_time = time.time()
    
    @task
    def get_directions(self) -> None:
        location = random.choice(SAMPLE_LOCATIONS)
        
        params: Dict[str, Any] = {
            "origin": location["origin"],
            "destination": location["destination"],
            "key": API_KEY,
            "mode": random.choice(["driving", "walking", "transit", "bicycling"]),
            "alternatives": random.choice(["true", "false"])
        }
        
        with self.client.get("/maps/api/directions/json", params=params, catch_response=True) as response:
            if response.status_code == 502:
                response.failure("502 Bad Gateway Error")
            elif response.status_code != 200:
                response.failure(f"HTTP Error: {response.status_code}")
            elif "error_message" in response.text:
                response.failure(f"API Error: {response.text[:100]}...")
            
            if response.elapsed.total_seconds() > 5:
                response.failure("Response too slow")

# Statistics trackers
stats: Dict[str, int] = {
    "requests": 0,
    "failures": 0,
    "status_502": 0
}

@events.request.add_listener
def on_request(request_type, name, response_time, response_length, exception, **kwargs) -> None:
    global stats
    stats["requests"] += 1
    
    if exception:
        stats["failures"] += 1
    elif kwargs.get("response"):
        if kwargs["response"].status_code == 502:
            stats["status_502"] += 1
            stats["failures"] += 1
        elif kwargs["response"].status_code >= 400:
            stats["failures"] += 1
    
    if stats["requests"] % 50 == 0:
        failure_rate = (stats["failures"] / stats["requests"]) * 100 if stats["requests"] > 0 else 0
        print(f"Total: {stats['requests']}, Failures: {stats['failures']} ({failure_rate:.1f}%), 502s: {stats['status_502']}")