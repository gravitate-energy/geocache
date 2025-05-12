import os
import requests
from dotenv import load_dotenv

def get_env(key: str, default: str | None = None) -> str | None:
    return os.getenv(key) or default

load_dotenv()

API_KEY = get_env('GOOGLE_MAPS_KEY', 'test-key')
BASE_URL = get_env('GOOGLE_MAPS_URL', 'http://localhost:8081/maps/api/directions/json')

ORIGIN = 'New York, NY'
DESTINATION = 'Boston, MA'

params = {
    'origin': ORIGIN,
    'destination': DESTINATION,
    'key': API_KEY,
}

for i in range(3):
    resp = requests.get(BASE_URL, params=params)
    x_cache = resp.headers.get('X-Cache')
    print(f'Request {i+1}: X-Cache={x_cache}, Status={resp.status_code}') 