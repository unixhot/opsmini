# systemd 호스팅

OpsMini를 systemd 서비스로 등록하여 부팅 자동 시작과 프로세스 보호를 구현합니다.

## 서비스 파일 생성

`/etc/systemd/system/opsmini.service`를 생성합니다:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# 보안 강화（선택 사항）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## 활성화 및 시작

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## 자주 쓰는 관리 명령

```bash
systemctl status opsmini    # 상태 조회
systemctl restart opsmini   # 재시작
systemctl stop opsmini      # 중지
journalctl -u opsmini -f    # 실시간 로그 조회
```

## systemd가 없는 환경

일부 컨테이너/경량 시스템에는 systemd가 없으므로 수동으로 시작할 수 있습니다:

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
