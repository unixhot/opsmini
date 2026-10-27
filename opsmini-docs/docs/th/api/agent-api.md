# การเชื่อมต่อ Agent (gRPC)

โปรเซสของ OpsMini มี **Agent client** อยู่ในตัว มันจะเปิด gRPC แบบสองทางหนึ่งเส้นออกไปเอง เพื่อลงทะเบียน ส่ง heartbeat รับงาน และรับส่งไฟล์ **ไม่ต้องให้ปลายทาง SSH ได้หรือเปิดพอร์ตขาเข้า** จึงเหมาะกับเครื่องที่อยู่หลัง NAT เครือข่ายภายใน หรือไม่มี IP สาธารณะ

!!! warning "เกี่ยวกับ REST API เดิม /agent/v1"
    `/agent/v1` REST API และหน้า "ตั้งค่า → API Token" ถูก**นำออกตั้งแต่ v1.2.1** หากต้องการสั่งงานจากระบบภายนอกให้ใช้ **MCP** (`/mcp` ดู [Panel API](browser-api.md)) หากต้องการจัดการหลายโฮสต์ให้ใช้ gRPC Agent ในหน้านี้

## รูปแบบการสื่อสาร

| ทิศทาง | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- พอร์ตเดียวพอ: HTTP/2 multiplexing ไม่ต้องใช้สองพอร์ตแบบ SaltStack
- หลุดแล้ว reconnect แบบ exponential backoff พร้อม heartbeat สม่ำเสมอ
- หากชื่อโฮสต์เดิมเชื่อมต่อใหม่ ระบบจะตัดการเชื่อมต่อเก่าทิ้ง

## การตั้งค่า

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

หากเว้น `server_addr` ว่าง จะปิดโหมด Agent และแผงยังทำงานแบบเครื่องเดียวได้ตามปกติ

## ขอบเขตความสามารถ

| ความสามารถ | รายละเอียด |
|------|------|
| รันคำสั่ง / สคริปต์ | `ExecuteTask` ใช้ oneof แยก command กับ script |
| ส่งไฟล์ลง | `PushFileTask` ส่งแบบ chunk ต่อเนื่อง รองรับ `resume_offset` และตรวจ md5 |
| ดึงไฟล์ขึ้น | Agent อ่านไฟล์แล้วส่งกลับ |
| เก็บค่าตัวชี้วัด | ไม่ผ่าน gRPC แต่เปิดที่ `/metrics` ให้ Prometheus ดึง |

!!! note "ตำแหน่งโค้ดฝั่งเซิร์ฟเวอร์"
    OpsMini Server อยู่ในโปรเจกต์ OpsAnt (`github.com/opsany/opsmini-server`) รีโปนี้มีเฉพาะฝั่ง Agent (`internal/agent/client.go`)
