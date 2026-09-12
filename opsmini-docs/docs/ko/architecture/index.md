# 아키텍처 설계

OpsMini의 기술 아키텍처와 설계 결정 사항.

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[전체 아키텍처](index.md)**

    ---

    단일 머신 패널, 모놀리식 단일 바이너리, 프런트엔드/백엔드 분리 + 임베드 패키징.

-   :material-cube-outline: **[기술 스택](tech-stack.md)**

    ---

    Go · Gin · GORM · SQLite（순수 Go）· Vue 3 · ECharts 선정 이유.

-   :material-folder-outline: **[디렉터리 구조](directory.md)**

    ---

    계층 아키텍처（Controller → Service → Repository）와 디렉터리 구성.

</div>

## 핵심 아키텍처 결정

- **단일 머신 모놀리식**: 마이크로서비스를 사용하지 않고 단일 프로세스 단일 바이너리로 제공
- **SQLite**: 임베디드 무운영(제로 오퍼레이션), 비즈니스 데이터량이 적어 충분히 감당 가능
- **이중 API**: `/api/v1`은 UI 대상, `/agent/v1`은 통합 대상, 인증과 권한 격리
- **순수 Go 무 CGO**: 임의 플랫폼에서 원클릭 크로스 컴파일로 정적 링크 바이너리 생성
