# System Management

The system management page provides a comprehensive view of host resources.

## Modules

| Module | Content |
|------|------|
| System info | hostname, OS, kernel version, uptime |
| Processes | Process list (PID, CPU, memory usage) |
| Network | Network interfaces and traffic |
| Ports | Port listening status |
| Disks | Disks and mount points, capacity usage |
| System users | System user list |
| User groups | User group list |
| Firewall | Firewall status and rules |

## Metric Sources

System resource information is collected by `gopsutil`, compatible across Linux / macOS (macOS is for development debugging only).
