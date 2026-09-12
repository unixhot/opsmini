# Architektur-Design

Technische Architektur und Designentscheidungen von OpsMini.

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[Gesamtarchitektur](index.md)**

    ---

    Einzel-Server-Panel, Monolith als einzelnes Binary, getrenntes Frontend/Backend + eingebettete Paketierung.

-   :material-cube-outline: **[Technologie-Stack](tech-stack.md)**

    ---

    Auswahlbegründung für Go · Gin · GORM · SQLite (reines Go) · Vue 3 · ECharts.

-   :material-folder-outline: **[Verzeichnisstruktur](directory.md)**

    ---

    Schichtarchitektur (Controller → Service → Repository) und Verzeichnisorganisation.

</div>

## Zentrale Architekturentscheidungen

- **Einzel-Server-Monolith**: keine Microservices, Auslieferung als einzelner Prozess und einzelnes Binary
- **SQLite**: eingebettet und wartungsfrei, für das geringe Geschäftsdatenvolumen vollkommen ausreichend
- **Doppelte API**: `/api/v1` für die UI, `/agent/v1` für Integration, getrennte Authentifizierung und Berechtigungen
- **Reines Go ohne CGO**: plattformübergreifendes Cross-Kompilieren statisch gelinkter Binaries mit einem Klick
