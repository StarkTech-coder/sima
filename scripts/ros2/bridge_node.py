#!/usr/bin/env python3
import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse

import rclpy
from rclpy.node import Node
from std_msgs.msg import String

MESSAGE_VERSION = 1


class BridgeNode(Node):
    def __init__(self):
        super().__init__('sima_bridge')
        self.telemetry = {}
        self.acks = {}
        self.lock = threading.Lock()
        self.cond = threading.Condition(self.lock)
        self.command_pub = self.create_publisher(String, '/sima/command', 50)
        self.create_subscription(String, '/sima/telemetry', self._on_telemetry, 50)
        self.create_subscription(String, '/sima/ack', self._on_ack, 50)

    def _on_telemetry(self, msg: String):
        try:
            payload = json.loads(msg.data)
            vehicle_id = payload['vehicle_id']
        except Exception:
            return
        with self.lock:
            self.telemetry[vehicle_id] = payload
            self.cond.notify_all()

    def _on_ack(self, msg: String):
        try:
            payload = json.loads(msg.data)
            key = f"{payload['vehicle_id']}:{payload['command_id']}:{payload['generation']}"
        except Exception:
            return
        with self.lock:
            self.acks[key] = payload
            self.cond.notify_all()

    def get_telemetry(self, vehicle_id: str):
        with self.lock:
            return self.telemetry.get(vehicle_id)

    def wait_ack(self, vehicle_id: str, command_id: str, generation: int, timeout: float):
        key = f"{vehicle_id}:{command_id}:{generation}"
        end = time.time() + timeout
        with self.lock:
            while True:
                ack = self.acks.get(key)
                if ack:
                    return ack
                remain = end - time.time()
                if remain <= 0:
                    return None
                self.cond.wait(remain)

    def publish_command(self, payload: dict):
        msg = String()
        msg.data = json.dumps(payload)
        self.command_pub.publish(msg)


class Handler(BaseHTTPRequestHandler):
    bridge = None

    def _send(self, code: int, payload: dict):
        data = json.dumps(payload).encode('utf-8')
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path.startswith('/v1/telemetry/'):
            vehicle_id = parsed.path.split('/')[-1]
            telemetry = self.bridge.get_telemetry(vehicle_id)
            if not telemetry:
                self._send(404, {'error': 'telemetry not found'})
                return
            self._send(200, telemetry)
            return
        self._send(404, {'error': 'not found'})

    def do_POST(self):
        if self.path != '/v1/command':
            self._send(404, {'error': 'not found'})
            return
        try:
            size = int(self.headers.get('Content-Length', '0'))
            payload = json.loads(self.rfile.read(size).decode('utf-8'))
        except Exception:
            self._send(400, {'error': 'invalid json'})
            return

        if payload.get('message_version') != MESSAGE_VERSION:
            self._send(400, {'error': 'unsupported version'})
            return
        if not payload.get('vehicle_id') or not payload.get('command_id'):
            self._send(400, {'error': 'missing identity'})
            return

        self.bridge.publish_command(payload)
        timeout = max(0.2, min(float(payload.get('timeout_ms', 2000)) / 1000.0, 120.0))
        ack = self.bridge.wait_ack(payload['vehicle_id'], payload['command_id'], int(payload.get('generation', 0)), timeout)
        if not ack:
            self._send(504, {'error': 'ack timeout'})
            return
        self._send(200, ack)


def spin_bridge(bridge: BridgeNode):
    rclpy.spin(bridge)


def main():
    rclpy.init()
    bridge = BridgeNode()
    Handler.bridge = bridge

    spin = threading.Thread(target=spin_bridge, args=(bridge,), daemon=True)
    spin.start()

    server = ThreadingHTTPServer(('0.0.0.0', 8090), Handler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.shutdown()
        bridge.destroy_node()
        rclpy.shutdown()


if __name__ == '__main__':
    main()
