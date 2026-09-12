# อัปเกรด

## ขั้นตอนการอัปเกรด

```bash
# 1. หยุดบริการและสำรองข้อมูล
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. แทนที่ไบนารี
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. รีสตาร์ท
sudo systemctl start opsmini
```

## หมายเหตุ

- SQLite ถูก GORM auto-migrate การอัปเกรดข้ามเวอร์ชันโดยปกติไม่ต้องแก้ตารางเอง
- ก่อนอัปเกรดต้องสำรอง `opsmini.db` ข้อมูลคือสถานะธุรกิจทั้งหมดของพาเนล
- หากเวอร์ชันใหม่เพิ่มรายการกำหนดค่า ต้องอัปเดต `config.yaml` ให้สอดคล้อง (อ้างอิงเทมเพลต `configs/config.yaml`)

## การย้อนกลับ

หากหลังอัปเกรดมีความผิดปกติ ให้แทนที่กลับเป็นไบนารีเก่าและกู้คืนข้อมูลสำรอง:

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # ใช้ไบนารีเก่า
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
