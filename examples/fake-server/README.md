# NVIDIA Device API: Simulated Server

This program provides a simulated NVIDIA Device API server for development and testing. It maintains an in-memory inventory of mock GPUs and periodically modifies their readiness to provide a dynamic data source without requiring physical hardware.

---

## Usage

Run the server in a dedicated terminal window:

```bash
# Default target: unix:///var/run/nvidia-device-api/device-api.sock
go run main.go
```
To stop the server, press `Ctrl+C`

> [!TIP]
> **Permissions**: If you do not have write access to `/var/run/`, override the socket path to run without root privileges: `export NVIDIA_DEVICE_API_TARGET="unix:///tmp/device-api.sock"`

---
