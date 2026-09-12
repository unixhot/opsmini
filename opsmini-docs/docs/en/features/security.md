# Host Security

OpsMini has a built-in host security center providing five major security capabilities.

## Security Overview

The security center home page aggregates the overall security posture, including baseline compliance, file changes, threat findings, firewall status, login security, and more.

## Baseline Checks

- Perform security baseline scans on the host (e.g. password policy, SSH configuration, system hardening items)
- View the latest scan results and non-compliant items
- Trigger a scan with one click

## File Integrity Monitoring (FIM)

- Establish a baseline for critical files / directories
- Detect file additions, deletions, and modifications and record events
- Support rebuilding the baseline and viewing change events

## Threat Detection

- Scan host threats (abnormal processes, suspicious files, etc.)
- View the threat findings list
- Mark threats as handled

## Firewall

- View firewall status
- Enable / disable the firewall
- Allow / deny ports and delete rules

## Login Security

- Analyze login behavior (success / failure, source IP)
- View SSH login records
- Work together with login failure rate limiting and lockout

## Related Permission Points

Security operations are controlled by RBAC permission points: `security.scan`, `security.firewall`, `security.fim`, `security.threat`.
