# Eureka Client - Hướng dẫn sử dụng

## Tổng quan

Service này hoạt động như một Eureka Client, tự động đăng ký với Eureka Server khi khởi động và hủy đăng ký khi shutdown.

## Kiến trúc

```
┌─────────────────────────────────────────────────────────────┐
│                     Eureka Server                           │
│                   (localhost:8761)                          │
└─────────────────────────────────────────────────────────────┘
         ▲                    │                    ▲
         │ Register           │ Dashboard          │ Heartbeat
         │ Deregister         ▼                    │ (30s)
         │              ┌───────────┐              │
         └──────────────│  Browser  │──────────────┘
                        └───────────┘
                              
┌─────────────────────────────────────────────────────────────┐
│              Docube Blockchain Service (Go)                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Config    │  │   Eureka    │  │      Main Loop      │  │
│  │   Loader    │→ │   Client    │→ │  (Block + Signals)  │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Luồng hoạt động

### 1. Khởi động (Startup)

```
1. Load configuration (YAML + Environment Variables)
2. Create Eureka Client instance
3. Register with Eureka Server (POST /apps/{appName})
4. Start heartbeat goroutine
5. Block main thread, wait for OS signals
```

### 2. Đang chạy (Running)

```
Every 30 seconds:
  - Send heartbeat (PUT /apps/{appName}/{instanceId})
  - If heartbeat fails: retry after 5s
  - If retry fails: attempt re-registration
```

### 3. Shutdown (Graceful)

```
On SIGINT/SIGTERM:
  1. Stop heartbeat goroutine
  2. Deregister from Eureka (DELETE /apps/{appName}/{instanceId})
  3. Exit cleanly
```

## Eureka REST API được sử dụng

| Action | Method | Endpoint |
|--------|--------|----------|
| Register | POST | `/eureka/apps/{appName}` |
| Heartbeat | PUT | `/eureka/apps/{appName}/{instanceId}` |
| Deregister | DELETE | `/eureka/apps/{appName}/{instanceId}` |

## Thông tin đăng ký

Khi đăng ký với Eureka, service gửi các thông tin sau:

```json
{
  "instance": {
    "instanceId": "hostname:fabric-gateway-service:8080",
    "hostName": "hostname",
    "app": "FABRIC-GATEWAY-SERVICE",
    "ipAddr": "192.168.x.x",
    "status": "UP",
    "port": { "$": 8080, "@enabled": "true" },
    "metadata": {
      "env": "dev",
      "language": "go",
      "role": "gateway"
    }
  }
}
```

## Cấu hình

### Qua YAML file (config/app.yaml)

```yaml
app:
  name: fabric-gateway-service
  port: 8080
  env: dev

eureka:
  server_url: http://localhost:8761/eureka
  heartbeat_interval: 30
  retry_interval: 5
```

### Qua Environment Variables

```bash
export APP_NAME=fabric-gateway-service
export APP_PORT=8080
export ENV=dev
export EUREKA_SERVER_URL=http://localhost:8761/eureka
export EUREKA_HEARTBEAT_INTERVAL=30
```

> **Ưu tiên:** Environment variables sẽ ghi đè giá trị trong YAML file.

## Chạy Service

### Yêu cầu

1. Go 1.21+
2. Eureka Server đang chạy tại `http://localhost:8761`

### Bước 1: Build

```bash
cd /home/horob1/docube_blockchain_service
go build -o bin/server ./cmd/server
```

### Bước 2: Chạy

**Cách 1: Sử dụng YAML config (mặc định)**
```bash
./bin/server
```

**Cách 2: Sử dụng Environment Variables**
```bash
APP_NAME=my-service APP_PORT=9090 EUREKA_SERVER_URL=http://eureka:8761/eureka ./bin/server
```

**Cách 3: Chạy trực tiếp với Go**
```bash
go run ./cmd/server
```

### Bước 3: Kiểm tra

1. Mở Eureka Dashboard: http://localhost:8761
2. Tìm service `FABRIC-GATEWAY-SERVICE` trong danh sách
3. Status phải là `UP`

### Bước 4: Shutdown

Nhấn `Ctrl+C` để shutdown gracefully. Service sẽ:
- Dừng heartbeat
- Hủy đăng ký khỏi Eureka
- Thoát

## Log mẫu

```
========================================
🚀 Docube Blockchain Service Starting...
========================================
📋 Configuration loaded:
   App Name: fabric-gateway-service
   App Port: 8080
   Environment: dev
   Eureka Server: http://localhost:8761/eureka
📡 Registering with Eureka server...
[EUREKA] ✅ Successfully registered: app=FABRIC-GATEWAY-SERVICE, instanceId=hostname:fabric-gateway-service:8080, ip=192.168.1.100:8080
[EUREKA] 💓 Starting heartbeat every 30s
========================================
✅ Service is running and registered!
   Press Ctrl+C to shutdown gracefully
========================================
[EUREKA] 💓 Heartbeat sent successfully
[EUREKA] 💓 Heartbeat sent successfully
^C
🛑 Received signal: interrupt - initiating graceful shutdown...
📡 Deregistering from Eureka...
[EUREKA] ✅ Successfully deregistered: app=FABRIC-GATEWAY-SERVICE, instanceId=hostname:fabric-gateway-service:8080
========================================
👋 Service shutdown complete. Goodbye!
========================================
```

## Troubleshooting

### Service không đăng ký được

1. Kiểm tra Eureka Server đang chạy: `curl http://localhost:8761/eureka/apps`
2. Kiểm tra URL trong config
3. Kiểm tra firewall/network

### Heartbeat thất bại

- Service sẽ tự động retry sau 5 giây
- Nếu retry thất bại, sẽ thử đăng ký lại

### Service không hiển thị trong Eureka Dashboard

- Đợi vài giây (Eureka có cache)
- Refresh trang Dashboard
- Kiểm tra log của service

## Cấu trúc thư mục

```
docube_blockchain_service/
├── cmd/server/main.go          # Entry point với Eureka lifecycle
├── config/app.yaml             # YAML configuration
├── internal/
│   ├── config/config.go        # Config loader (YAML + ENV)
│   └── eureka/client.go        # Eureka REST client
├── .env.example                # Environment template
└── go.mod                      # Go module
```

## Roadmap (Future)

- [ ] Fabric SDK integration
- [ ] REST API endpoints
- [ ] gRPC endpoints
- [ ] Service-to-service discovery via Eureka
