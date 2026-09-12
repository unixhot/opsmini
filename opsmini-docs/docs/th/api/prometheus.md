# Prometheus metrics (/metrics)

OpsMini มีเอนด์พอยต์ `/metrics` ที่**เข้ากันได้กับ node_exporter** ในตัว ไม่จำเป็นต้องติดตั้ง node_exporter เพิ่ม Prometheus สามารถ scrape ได้โดยตรง

## การเปิดใช้งานและการยืนยันตัวตน

เปิดใช้งานใน `config.yaml` (เปิดใช้งานเป็นค่าเริ่มต้น):

```yaml
metrics:
  enabled: true      # เปิดใช้งานเอนด์พอยต์ /metrics หรือไม่
  user: ""           # ชื่อผู้ใช้ Basic authentication เว้นว่างหมายถึงไม่มีการยืนยันตัวตน
  password: ""       # รหัสผ่าน Basic authentication
```

ลำดับความสำคัญของข้อมูลรับรอง: ชื่อผู้ใช้/รหัสผ่านใน "การตั้งค่า → การส่งออกมอนิเตอร์" ของพาเนล **มีลำดับความสำคัญสูงกว่า** ไฟล์กำหนดค่า; หากปล่อยชื่อผู้ใช้ว่างหมายถึงไม่มีการยืนยันตัวตน

## การกำหนดค่า Prometheus

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # หากเปิดใช้การยืนยันตัวตน ต้องกำหนดข้อมูลรับรองที่ตรงกัน
      username: 'monitor'
      password: '<password>'
```

## กลุ่มเมตริก

ได้จัดแนวเมตริกหลักของ node_exporter แล้ว สามารถใช้ Node Dashboard ของชุมชนได้โดยตรง:

| กลุ่มเมตริก | คำอธิบาย |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | วินาทีสะสมของแต่ละโหมดต่อคอร์ |
| `node_memory_MemTotal_bytes` ฯลฯ | หน่วยความจำ Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | ความจุ / ที่ว่าง / อัตราการใช้งานของแต่ละจุด mount |
| `node_network_receive_bytes_total{device}` | ปริมาณการรับส่งข้อมูลของแต่ละการ์ดเครือข่าย |
| `node_load1` / `node_load5` / `node_load15` | โหลด |
| `node_uname_info` / `node_boot_time_seconds` | ข้อมูลโฮสต์และเวลาเริ่มระบบ |

## หมายเหตุ

- การใช้งานใช้ข้อมูลที่ `gopsutil` เก็บมาแล้ว นำออกในรูปแบบข้อความ Prometheus
- คงการส่งมอบแบบไบนารีเดียว ไม่ฝังโปรเซส node_exporter
- หากกำหนดค่า `server.secret_entry` เอนด์พอยต์จะอยู่ภายใต้คำนำหน้าเดียวกัน (เช่น `/opsmini_panel/metrics`)
