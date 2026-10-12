"""Checks the synthetic CoreScope/terrain contract without Docker or a live mesh."""
import hashlib
import hmac
import io
import json
import struct
import threading
import unittest
import urllib.error
import urllib.request
import zlib
from datetime import datetime, timezone
from http.server import ThreadingHTTPServer
from unittest.mock import patch

import server


class FixtureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.httpd = ThreadingHTTPServer(("127.0.0.1", 0), server.Handler)
        cls.thread = threading.Thread(target=cls.httpd.serve_forever, daemon=True)
        cls.thread.start()
        cls.base = "http://127.0.0.1:%d" % cls.httpd.server_port

    @classmethod
    def tearDownClass(cls):
        cls.httpd.shutdown()
        cls.thread.join()
        cls.httpd.server_close()

    def get(self, path):
        with urllib.request.urlopen(self.base + path) as response:
            return json.load(response)

    def test_nodes_page_and_reach_use_only_synthetic_keys(self):
        nodes = self.get("/api/nodes?role=repeater")
        self.assertEqual(nodes["total"], 6)
        self.assertFalse(nodes["has_more"])
        page = self.get("/api/nodes?limit=2&offset=2")
        self.assertEqual(page["nodes"], nodes["nodes"][2:4])
        self.assertTrue(page["has_more"])
        self.assertEqual(self.get("/api/nodes?role=companion")["nodes"], [])
        keys = {node["public_key"] for node in nodes["nodes"]}
        self.assertEqual(len(keys), 6)
        for node in nodes["nodes"]:
            self.assertTrue(node["name"].startswith("Synthetic "))
            links = self.get("/api/nodes/%s/reach" % node["public_key"])["links"]
            self.assertEqual({link["pubkey"] for link in links}, keys - {node["public_key"]})
            self.assertTrue(all(link["bidir"] and link["bottleneck"] > 0 for link in links))

    def test_packet_windows_have_stable_replayable_scoped_and_unscoped_data(self):
        page = self.get("/api/packets?limit=200")
        self.assertEqual(page, self.get("/api/packets?limit=200"))
        packets = page["packets"]
        self.assertGreaterEqual(len(packets), 6)
        times = [datetime.fromisoformat(p["timestamp"]) for p in packets]
        self.assertEqual(times, sorted(times, reverse=True))
        self.assertLess((datetime.now(timezone.utc) - times[0]).total_seconds(), 3600)
        self.assertEqual(self.get("/api/packets?limit=1&offset=2")["packets"], packets[2:3])
        self.assertEqual(self.get("/api/packets?offset=1000")["packets"], [])
        self.assertEqual({p["route_type"] for p in packets}, {0, 1})
        names = [r["name"] for r in self.get("/api/scope-stats")["byRegion"]]
        for packet in packets:
            detail = self.get("/api/packets/" + packet["hash"])
            self.assertEqual(detail["packet"], packet)
            self.assertGreater(len(detail["observations"]), 1)
            self.assertTrue(all(o["resolved_path"] for o in detail["observations"]))
            raw = bytes.fromhex(packet["raw_hex"])
            offset = 5 if packet["route_type"] == 0 else 1
            count, width = raw[offset] & 63, (raw[offset] >> 6) + 1
            self.assertEqual(count, len(packet["resolved_path"]))
            if packet["route_type"] == 0:
                message = bytes([(raw[0] >> 2) & 15]) + raw[offset + 1 + count * width:]
                matches = [n for n in names if hmac.new(hashlib.sha256(n.encode()).digest()[:16], message, "sha256").digest()[:2] == raw[1:3]]
                self.assertEqual(matches, [packet["scope"]])

    def test_terrain_is_a_valid_flat_100_metre_terrarium_tile(self):
        with urllib.request.urlopen(self.base + "/terrarium/11/564/748.png") as response:
            self.assertEqual(response.headers["Content-Type"], "image/png")
            png = response.read()
        self.assertEqual(png[:8], b"\x89PNG\r\n\x1a\n")
        data, pos = b"", 8
        while pos < len(png):
            size = struct.unpack(">I", png[pos:pos + 4])[0]
            tag, body = png[pos + 4:pos + 8], png[pos + 8:pos + 8 + size]
            crc = struct.unpack(">I", png[pos + 8 + size:pos + 12 + size])[0]
            self.assertEqual(crc, zlib.crc32(tag + body))
            if tag == b"IHDR":
                self.assertEqual(struct.unpack(">IIBBBBB", body), (256, 256, 8, 2, 0, 0, 0))
            if tag == b"IDAT":
                data += body
            pos += size + 12
        pixels = zlib.decompress(data)
        self.assertEqual(pixels, (b"\x00" + bytes([128, 100, 0]) * 256) * 256)

    def test_unknown_endpoints_and_invalid_paging_fail_closed(self):
        for path, status in [("/api/unknown", 404), ("/api/nodes?offset=-1", 400),
                             ("/api/packets?limit=bad", 400), ("/api/packets/missing", 404),
                             ("/terrarium/1/8/0.png", 404)]:
            with self.assertRaises(urllib.error.HTTPError) as caught:
                self.get(path)
            self.assertEqual(caught.exception.code, status)

    def test_readiness_requires_completed_real_tiers_and_both_scopes(self):
        tiers = {name: {"tiles": ["fixture"]} for name in
                 ("standard", "calibrated", "precision", "calibrated_precision")}
        meta = {"complete": True, "coverage": tiers,
                "scope_coverage": {scope: {"tiles": ["fixture"]} for scope in server.SCOPES}}
        cases = [(meta, True), ({**meta, "complete": False}, False),
                 ({**meta, "coverage": {"standard": tiers["standard"]}}, False),
                 ({**meta, "scope_coverage": {}}, False)]
        for data, expected in cases:
            with self.subTest(data=data):
                body = io.BytesIO(json.dumps(data).encode())
                with patch.object(server.urllib.request, "urlopen", return_value=body):
                    self.assertEqual(server.readiness(), expected)


if __name__ == "__main__":
    unittest.main()
