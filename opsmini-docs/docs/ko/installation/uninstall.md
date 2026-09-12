# 제거

## 서비스 중지 및 제거

```bash
# systemd 서비스 중지 및 비활성화
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## 바이너리와 설정 제거

```bash
# 설치 디렉터리 제거（기본 /data/opsmini, 또는 사용자 지정 디렉터리）
sudo rm -rf /data/opsmini
# 또는
sudo rm -rf /data/opsmini /data/opsmini
```

## 전용 사용자 제거（선택 사항）

```bash
sudo userdel opsmini
```

## 설명

- 제거는 호스트 머신의 Docker 컨테이너 / 이미지, Nginx 사이트, 데이터베이스 등 패널이 관리하는 리소스에 영향을 주지 않으며, 이러한 리소스는 별도로 처리해야 합니다
- 완전히 정리하려면 `/data/opsmini/opsmini.db`에 보존해야 할 비즈니스 데이터가 없는지 확인하세요
