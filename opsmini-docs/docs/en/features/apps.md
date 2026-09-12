# App Management

App management centrally handles four types of resources: websites, databases, containers, and the app store.

## Websites

- Create / manage Nginx sites and static sites
- Configure domain names, runtime environments (PHP / Node / static), SSL certificate issuance and renewal
- Start/stop sites and view their status

## Databases

- Create / manage MySQL and PostgreSQL database instances
- View database lists and settings such as charset

## Containers (Docker)

- **Containers**: list, start, stop, restart, delete
- **Images**: view and delete images
- **Volumes**: view and delete volumes
- **Networks**: view and delete networks

> Docker management requires Docker to be installed on the host and `DOCKER_HOST` configured; if the Docker client fails to initialize, the panel gracefully degrades and hides the container routes.

## App Store

- Built-in common app categories (web services, databases, caches, monitoring, etc.)
- One-click install / uninstall / start / stop / restart for apps (Docker shell scripts)
- Supports custom app template import and official template sync
