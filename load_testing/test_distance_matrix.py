import os
import requests
from dotenv import load_dotenv

def get_env(key: str, default: str | None = None) -> str | None:
    return os.getenv(key) or default

load_dotenv()

API_KEY = get_env('GOOGLE_MAPS_KEY', 'test-key')
BASE_URL = get_env('GOOGLE_MAPS_URL', 'http://localhost:8081/maps/api/distancematrix/json')

locations = [
    (40.7128, -74.0060),  # New York
    (42.3601, -71.0589),  # Boston
    (41.8781, -87.6298),  # Chicago
]
gps_lookup = [f"{lat:.4f},{lon:.4f}" for lat, lon in locations]

params = {
    'origins': '|'.join(gps_lookup),
    'destinations': '|'.join(gps_lookup),
    'key': API_KEY,
}

for i in range(3):
    resp = requests.get(BASE_URL, params=params)
    x_cache = resp.headers.get('X-Cache')
    print(f'Request {i+1}: X-Cache={x_cache}, Status={resp.status_code}') 