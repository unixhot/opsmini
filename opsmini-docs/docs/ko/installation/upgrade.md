# 업그레이드

## 업그레이드 단계

```bash
# 1. 서비스 중지 및 데이터 백업
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. 바이너리 교체
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. 재시작
sudo systemctl start opsmini
```

## 설명

- SQLite는 GORM이 자동 마이그레이션하므로 버전 간 업그레이드 시 일반적으로 수동 테이블 변경이 필요 없습니다
- 업그레이드 전에 반드시 `opsmini.db`를 백업하세요. 데이터가 곧 패널의 전체 비즈니스 상태입니다
- 새 버전에서 설정 항목이 도입되면 `config.yaml`을 동기화하여 업데이트해야 합니다（`configs/config.yaml` 템플릿 참조）

## 롤백

업그레이드 후 이상이 있으면 이전 바이너리로 교체하고 데이터 백업을 복원합니다:

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # 이전 바이너리 사용
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
