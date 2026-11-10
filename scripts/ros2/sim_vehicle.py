#!/usr/bin/env python3
import json
import math
import os
import time

import rclpy
from rclpy.node import Node
from std_msgs.msg import String

MESSAGE_VERSION = 1


class VehicleNode(Node):
    def __init__(self):
        node_suffix = os.getenv('SIMA_VEHICLE_ID', 'AUV-01').lower().replace('-', '_')
        super().__init__('sima_vehicle_' + node_suffix)
        self.vehicle_id = os.getenv('SIMA_VEHICLE_ID', 'AUV-01')
        self.battery = float(os.getenv('SIMA_BATTERY', '92'))
        self.connected = True
        self.epoch = f"{self.vehicle_id}-epoch"
        self.sequence = 0
        self.x = float(os.getenv('SIMA_X', '10'))
        self.y = float(os.getenv('SIMA_Y', '20'))
        self.mode = 'ready'
        self.assignments = {}
        self.command_cache = {}
        self.disconnect_after = float(os.getenv('SIMA_DISCONNECT_AFTER', '0'))
        self.disconnect_duration = float(os.getenv('SIMA_DISCONNECT_DURATION', '0'))
        self.started = time.time()

        self.telemetry_pub = self.create_publisher(String, '/sima/telemetry', 50)
        self.ack_pub = self.create_publisher(String, '/sima/ack', 50)
        self.create_subscription(String, '/sima/command', self._on_command, 50)
        self.create_timer(0.5, self._tick)

    def _is_connected(self):
        if self.disconnect_after <= 0 or self.disconnect_duration <= 0:
            return True
        elapsed = time.time() - self.started
        return not (self.disconnect_after <= elapsed < self.disconnect_after + self.disconnect_duration)

    def _publish_ack(self, command, state, accepted, reason=''):
        payload = {
            'vehicle_id': self.vehicle_id,
            'message_id': f"ack-{command['command_id']}-{time.time_ns()}",
            'message_version': MESSAGE_VERSION,
            'timestamp': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
            'command_id': command['command_id'],
            'task_id': command.get('task_id', ''),
            'generation': int(command.get('generation', 0)),
            'state': state,
            'accepted': bool(accepted),
            'reason': reason,
        }
        msg = String()
        msg.data = json.dumps(payload)
        self.ack_pub.publish(msg)
        self.command_cache[command['command_id']] = payload

    def _on_command(self, msg: String):
        if not self.connected:
            return
        try:
            command = json.loads(msg.data)
        except Exception:
            return
        if command.get('vehicle_id') != self.vehicle_id:
            return
        if command.get('message_version') != MESSAGE_VERSION:
            self._publish_ack(command, 'rejected', False, 'unsupported version')
            return
        if command.get('command_id') in self.command_cache:
            cached = self.command_cache[command['command_id']]
            out = String()
            out.data = json.dumps(cached)
            self.ack_pub.publish(out)
            return

        self._publish_ack(command, 'received', True)
        kind = command.get('kind', '')
        task_id = command.get('task_id', '')
        generation = int(command.get('generation', 0))

        if generation <= 0 or not task_id:
            self._publish_ack(command, 'rejected', False, 'invalid command identity')
            return

        if kind == 'assign':
            status = 'running' if command.get('start', False) else 'queued'
            self.assignments[task_id] = {'generation': generation, 'status': status, 'progress': 0.0, 'result_acked': False}
            self.mode = 'surveying' if status == 'running' else 'ready'
            self._publish_ack(command, 'accepted', True, 'accepted')
            return

        assignment = self.assignments.get(task_id)
        if not assignment or assignment['generation'] != generation:
            self._publish_ack(command, 'rejected', False, 'assignment generation mismatch')
            return

        if kind == 'start':
            if assignment['status'] != 'queued':
                self._publish_ack(command, 'rejected', False, 'not queued')
                return
            assignment['status'] = 'running'
            self.mode = 'surveying'
            self._publish_ack(command, 'accepted', True, 'accepted')
            return

        if kind == 'release':
            if assignment['status'] not in ('queued', 'released'):
                self._publish_ack(command, 'rejected', False, 'only unstarted work can be released')
                return
            assignment['status'] = 'released'
            self._publish_ack(command, 'accepted', True, 'released')
            return

        if kind == 'result-ack':
            if assignment['status'] != 'completed':
                self._publish_ack(command, 'rejected', False, 'no completed result')
                return
            assignment['result_acked'] = True
            self._publish_ack(command, 'accepted', True, 'result-recorded')
            return

        self._publish_ack(command, 'failed', False, f"unknown command {kind}")

    def _tick(self):
        self.connected = self._is_connected()
        for task_id, assignment in list(self.assignments.items()):
            if assignment['status'] != 'running':
                continue
            assignment['progress'] = min(1.0, assignment['progress'] + 0.03)
            self.battery = max(0.0, self.battery - 0.1)
            self.x += 0.8
            self.y += math.sin(assignment['progress'] * math.pi) * 0.2
            if assignment['progress'] >= 1.0:
                assignment['status'] = 'completed'
                self.mode = 'segment-complete'
                self._publish_ack({'command_id': f"complete-{task_id}-{assignment['generation']}", 'task_id': task_id, 'generation': assignment['generation']}, 'completed', True, 'completed')

        if not self.connected:
            return

        self.sequence += 1
        payload = {
            'vehicle_id': self.vehicle_id,
            'message_id': f"tm-{self.vehicle_id}-{self.sequence}",
            'message_version': MESSAGE_VERSION,
            'timestamp': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
            'epoch': self.epoch,
            'sequence': self.sequence,
            'position': {'x': self.x, 'y': self.y},
            'depth': 65.0,
            'battery': self.battery,
            'speed': 1.2,
            'mode': self.mode,
            'connected': True,
            'assignments': [
                {
                    'task': {'id': tid, 'name': tid, 'capability': 'survey', 'minBattery': 25, 'duration': 80, 'route': [{'x': self.x, 'y': self.y}, {'x': self.x + 1, 'y': self.y + 1}]},
                    'generation': data['generation'],
                    'status': data['status'],
                    'progress': data['progress'],
                    'resultAcked': data['result_acked'],
                }
                for tid, data in sorted(self.assignments.items())
            ],
        }
        msg = String()
        msg.data = json.dumps(payload)
        self.telemetry_pub.publish(msg)


def main():
    rclpy.init()
    node = VehicleNode()
    try:
        rclpy.spin(node)
    except KeyboardInterrupt:
        pass
    finally:
        node.destroy_node()
        rclpy.shutdown()


if __name__ == '__main__':
    main()
