# internal/pkg/store

SQLite connection, auto-migration and seed data.

Uses the pure-Go driver `glebarez/sqlite` (no CGO). `Init` opens the
database, migrates all models, and seeds the default `admin` account
(initial password `opsmini`).
