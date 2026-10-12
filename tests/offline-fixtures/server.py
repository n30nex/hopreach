"""Synthetic CoreScope API + flat Terrarium DEM; never contacts a public service."""
import hashlib
import hmac
import json
import os
import re
import struct
import urllib.request
import zlib
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit


# Anchor once per process: pagination and packet detail never drift between calls.
EPOCH = datetime.now(timezone.utc).replace(microsecond=0) - timedelta(seconds=90)
SCOPES = ["#fixture-west", "#fixture-east"]
POSITIONS = [(43.43, -80.49), (43.50, -80.49), (43.57, -80.49),
             (43.43, -80.38), (43.50, -80.38), (43.57, -80.38)]
KEYS = [f"{index + 1:02x}" * 32 for index in range(len(POSITIONS))]


def timestamp(seconds=0):
    return (EPOCH + timedelta(seconds=seconds)).isoformat().replace("+00:00", "Z")


def make_nodes():
    return [{"public_key": key, "name": f"Synthetic Repeater {i + 1}",
             "role": "repeater", "lat": POSITIONS[i][0], "lon": POSITIONS[i][1],
             "last_heard": timestamp(), "first_seen": timestamp(-86400),
             "advert_count": 12, "relay_count_1h": 20, "relay_count_24h": 100,
             "hash_size": 1, "default_scope": SCOPES[i // 3]}
            for i, key in enumerate(KEYS)]


def make_packet(index):
    # Synthetic frames follow the contract pinned by internal/corescope/frame.go
    # and scope.go. Payload bytes are invented; no captured message data is used.
    path = KEYS[:3] if index % 2 == 0 else KEYS[3:]
    scope = SCOPES[index % 2] if index < 6 else ""
    payload_type, route = 5, 0 if scope else 1
    payload = f"synthetic-fixture-{index}".encode()
    raw = bytes([(payload_type << 2) | route])
    if scope:
        key = hashlib.sha256(scope.encode()).digest()[:16]
        code = hmac.new(key, bytes([payload_type]) + payload, "sha256").digest()[:2]
        raw += code + code
    raw += bytes([len(path)]) + bytes.fromhex("".join(key[:2] for key in path)) + payload
    return {"hash": hashlib.sha256(payload).hexdigest(), "timestamp": timestamp(-index * 5),
            "raw_hex": raw.hex(), "route_type": route, "observer_id": path[-1],
            "resolved_path": path, "_parsedPath": [key[:2] for key in path],
            "snr": 10, "decoded_json": json.dumps({"pubKey": path[0]}),
            "observation_count": 2, "scope": scope}


NODES = make_nodes()
PACKETS = [make_packet(i) for i in range(8)]


def packet_detail(packet):
    path = packet["resolved_path"]
    observations = [{"observer_id": path[i], "observer_name": NODES[KEYS.index(path[i])]["name"],
                     "timestamp": packet["timestamp"], "resolved_path": path[:i], "snr": 10}
                    for i in (1, 2)]
    return {"packet": packet, "observations": observations}


def reach(key):
    return {"links": [{"pubkey": node["public_key"], "name": node["name"],
                       "lat": node["lat"], "lon": node["lon"], "bottleneck": 20,
                       "bidir": True, "we_hear": 20, "they_hear": 24}
                      for node in NODES if node["public_key"] != key]}


def paginate(rows, query, label):
    limit = int(query.get("limit", [500])[0])
    offset = int(query.get("offset", [0])[0])
    if not 1 <= limit <= 10000 or offset < 0:
        raise ValueError("limit must be 1..10000 and offset nonnegative")
    return {label: rows[offset:offset + limit], "total": len(rows),
            "has_more": offset + limit < len(rows)}


def terrain_png():
    def chunk(tag, body):
        return struct.pack(">I", len(body)) + tag + body + struct.pack(">I", zlib.crc32(tag + body))
    # Terrarium: (R * 256 + G + B / 256) - 32768 = 100 metres.
    pixels = (b"\x00" + bytes([128, 100, 0]) * 256) * 256
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 256, 256, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(pixels)) + chunk(b"IEND", b""))


TERRAIN = terrain_png()


def readiness():
    # Read the real pipeline's output, never serve fabricated HopReach coverage.
    with urllib.request.urlopen("http://hopreach/data/meta.json", timeout=2) as response:
        meta = json.load(response)
    tiers = ("standard", "calibrated", "precision", "calibrated_precision")
    complete = meta.get("complete") and all(meta.get("coverage", {}).get(t) for t in tiers)
    return bool(complete and all(meta.get("scope_coverage", {}).get(s) for s in SCOPES))


class Handler(BaseHTTPRequestHandler):
    def send(self, status, body, content_type="application/json"):
        if content_type == "application/json":
            body = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        parsed = urlsplit(self.path)
        path, query = parsed.path, parse_qs(parsed.query)
        try:
            if path == "/health":
                return self.send(200, {"synthetic": True})
            if path == "/ready":
                return self.send(200 if readiness() else 503, {"synthetic": True})
            if path == "/api/nodes":
                role = query.get("role", ["repeater"])[0]
                return self.send(200, paginate(NODES if role == "repeater" else [], query, "nodes"))
            if path == "/api/scope-stats":
                return self.send(200, {"byRegion": [{"name": s} for s in SCOPES]})
            if path == "/api/packets":
                return self.send(200, paginate(PACKETS, query, "packets"))
            if path.startswith("/api/packets/"):
                packet = next((p for p in PACKETS if p["hash"] == path.rsplit("/", 1)[-1]), None)
                if packet:
                    return self.send(200, packet_detail(packet))
            if path.startswith("/api/nodes/") and path.endswith("/reach"):
                key = path.split("/")[3]
                if key in KEYS:
                    return self.send(200, reach(key))
            tile = re.fullmatch(r"/terrarium/(\d{1,2})/(\d{1,6})/(\d{1,6})\.png", path)
            if tile:
                zoom, x, y = map(int, tile.groups())
                if zoom <= 16 and x < 2 ** zoom and y < 2 ** zoom:
                    return self.send(200, TERRAIN, "image/png")
            self.send(404, {"error": "unknown fixture resource"})
        except ValueError as error:
            self.send(400, {"error": str(error)})
        except OSError:
            self.send(503, {"error": "HopReach coverage is not ready"})

    def log_message(self, *_args):
        pass


if __name__ == "__main__":
    address = ("0.0.0.0", int(os.environ.get("PORT", "8000")))
    print(f"Synthetic fixture listening on {address}, epoch {timestamp()}", flush=True)
    ThreadingHTTPServer(address, Handler).serve_forever()
