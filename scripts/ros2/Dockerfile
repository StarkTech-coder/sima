FROM ros:humble-ros-base

RUN apt-get update && apt-get install -y --no-install-recommends \
    ros-humble-rclpy \
    ros-humble-std-msgs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY bridge_node.py sim_vehicle.py /workspace/

ENV PYTHONUNBUFFERED=1
