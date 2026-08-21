# internal/pkg/response

Unified API response envelope.

All endpoints return `{ "code": 0, "message": "ok", "data": ... }`, where
a non-zero `code` indicates a business error.
