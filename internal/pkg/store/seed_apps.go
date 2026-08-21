// Package store handles SQLite initialization, migration, and seed data.
package store

import (
	"github.com/opsmini/opsmini/internal/model"
)

// seedAppList returns the default apps for the software store.
// Each app is a Shell script that starts with docker run in one click (modeled after the 1Panel app store).
// Scripts specify the data directory via the DATA_DIR environment variable (default /var/lib/opsmini/data).
func seedAppList() []model.App {
	scriptHeader := "#!/bin/bash\nset -e\nDATA_DIR=\"${DATA_DIR:-/var/lib/opsmini/data}\"\n"
	return []model.App{
		{
			Slug: "nginx", Name: "Nginx", Description: "高性能 Web 服务器与反向代理",
			Category: "web", Icon: "N", Color: "#16b879", Version: "1.25-alpine", Ports: "80:80, 443:443", ContainerName: "nginx",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/nginx/html\" \"$DATA_DIR/nginx/conf\"\n" +
				"docker rm -f nginx >/dev/null 2>&1 || true\n" +
				"docker run -d --name nginx --restart unless-stopped \\\n" +
				"  -p 80:80 -p 443:443 \\\n" +
				"  -v \"$DATA_DIR/nginx/html:/usr/share/nginx/html\" \\\n" +
				"  -v \"$DATA_DIR/nginx/conf:/etc/nginx/conf.d\" \\\n" +
				"  nginx:1.25-alpine\n",
		},
		{
			Slug: "redis", Name: "Redis", Description: "高性能内存键值缓存",
			Category: "cache", Icon: "R", Color: "#f05050", Version: "7-alpine", Ports: "6379:6379", ContainerName: "redis",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/redis\"\n" +
				"docker rm -f redis >/dev/null 2>&1 || true\n" +
				"docker run -d --name redis --restart unless-stopped \\\n" +
				"  -p 6379:6379 \\\n" +
				"  -v \"$DATA_DIR/redis:/data\" \\\n" +
				"  redis:7-alpine redis-server --appendonly yes\n",
		},
		{
			Slug: "mysql", Name: "MySQL", Description: "最流行的开源关系型数据库",
			Category: "database", Icon: "M", Color: "#3b9cf0", Version: "8.0", Ports: "3306:3306", ContainerName: "mysql",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/mysql\"\n" +
				"docker rm -f mysql >/dev/null 2>&1 || true\n" +
				"docker run -d --name mysql --restart unless-stopped \\\n" +
				"  -p 3306:3306 \\\n" +
				"  -e MYSQL_ROOT_PASSWORD=opsmini \\\n" +
				"  -v \"$DATA_DIR/mysql:/var/lib/mysql\" \\\n" +
				"  mysql:8.0\n",
		},
		{
			Slug: "mongodb", Name: "MongoDB", Description: "文档型 NoSQL 数据库",
			Category: "database", Icon: "DB", Color: "#47a248", Version: "7", Ports: "27017:27017", ContainerName: "mongodb",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/mongodb\"\n" +
				"docker rm -f mongodb >/dev/null 2>&1 || true\n" +
				"docker run -d --name mongodb --restart unless-stopped \\\n" +
				"  -p 27017:27017 \\\n" +
				"  -v \"$DATA_DIR/mongodb:/data/db\" \\\n" +
				"  mongo:7\n",
		},
		{
			Slug: "postgresql", Name: "PostgreSQL", Description: "功能强大的开源关系型数据库",
			Category: "database", Icon: "Pg", Color: "#336791", Version: "16-alpine", Ports: "5432:5432", ContainerName: "postgres",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/postgres\"\n" +
				"docker rm -f postgres >/dev/null 2>&1 || true\n" +
				"docker run -d --name postgres --restart unless-stopped \\\n" +
				"  -p 5432:5432 \\\n" +
				"  -e POSTGRES_PASSWORD=opsmini \\\n" +
				"  -v \"$DATA_DIR/postgres:/var/lib/postgresql/data\" \\\n" +
				"  postgres:16-alpine\n",
		},
		{
			Slug: "rabbitmq", Name: "RabbitMQ", Description: "消息队列中间件（含管理面板）",
			Category: "tools", Icon: "Rq", Color: "#ff6600", Version: "3-management", Ports: "5672:5672, 15672:15672", ContainerName: "rabbitmq",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/rabbitmq\"\n" +
				"docker rm -f rabbitmq >/dev/null 2>&1 || true\n" +
				"docker run -d --name rabbitmq --restart unless-stopped \\\n" +
				"  -p 5672:5672 -p 15672:15672 \\\n" +
				"  -v \"$DATA_DIR/rabbitmq:/var/lib/rabbitmq\" \\\n" +
				"  rabbitmq:3-management\n",
		},
		{
			Slug: "portainer", Name: "Portainer", Description: "Docker 可视化管理工具",
			Category: "tools", Icon: "P", Color: "#0db7ed", Version: "latest", Ports: "9000:9000", ContainerName: "portainer",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/portainer\"\n" +
				"docker rm -f portainer >/dev/null 2>&1 || true\n" +
				"docker run -d --name portainer --restart unless-stopped \\\n" +
				"  -p 9000:9000 \\\n" +
				"  -v /var/run/docker.sock:/var/run/docker.sock \\\n" +
				"  -v \"$DATA_DIR/portainer:/data\" \\\n" +
				"  portainer/portainer-ce:latest\n",
		},
		{
			Slug: "grafana", Name: "Grafana", Description: "监控可视化仪表盘",
			Category: "monitoring", Icon: "Gr", Color: "#f46800", Version: "latest", Ports: "3000:3000", ContainerName: "grafana",
			InstallScript: scriptHeader +
				"mkdir -p \"$DATA_DIR/grafana\"\n" +
				"docker rm -f grafana >/dev/null 2>&1 || true\n" +
				"docker run -d --name grafana --restart unless-stopped \\\n" +
				"  -p 3000:3000 \\\n" +
				"  -v \"$DATA_DIR/grafana:/var/lib/grafana\" \\\n" +
				"  grafana/grafana:latest\n",
		},
	}
}
