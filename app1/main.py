import os
import json
import redis

from flask import Flask, jsonify
from datetime import datetime
from zoneinfo import ZoneInfo
from prometheus_flask_exporter import PrometheusMetrics
from prometheus_client import Counter

app = Flask(__name__)
metrics = PrometheusMetrics(app)

redis_host = os.getenv("REDIS_HOST", "localhost")
redis_port = int(os.getenv("REDIS_PORT", "6379"))

r = redis.Redis(host=redis_host, port=redis_port, decode_responses=True)

cache_hits = Counter(
    "cache_hits_total",
    "Total de cache hits",
    ["app"]
)

cache_misses = Counter(
    "cache_misses_total",
    "Total de cache misses",
    ["app"]
)

@app.route("/time")
def getTime():
    cache_key = "app1:time"
    cached = r.get(cache_key)

    if cached:
        data = json.loads(cached)
        data["cache"] = "hit"
        return jsonify(data)

    sao_paulo_tz = ZoneInfo("America/Sao_Paulo")
    now = datetime.now(sao_paulo_tz)

    data = {
        "timezone": "America/Sao_Paulo",
        "current_time": now.strftime("%Y-%m-%d %H:%M:%S")
    }

    r.setex(cache_key, 10, json.dumps(data))
    data["cache"] = "miss"
    return jsonify(data)

@app.route("/text")
def getText():
    cache_key = "app1:text"
    cached = r.get(cache_key)

    if cached:
        data = json.loads(cached)
        data["cache"] = "hit"
        return jsonify(data)

    data = {
        "message": "Microservice activated: Python"
    }

    r.setex(cache_key, 10, json.dumps(data))
    data["cache"] = "miss"
    return jsonify(data)

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
