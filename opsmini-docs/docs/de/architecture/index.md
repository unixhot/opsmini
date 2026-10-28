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

-   :material-server-network: **[Backend-Architektur](backend.md)**

    ---

    Modulschnitt, Schichten und Verzeichnisse, Datenmodell, API-Design, Kernabläufe und Deployment.

-   :material-robot-outline: **[AI-Agent-Design](ai-agent.md)**

    ---

    Selbstgebauter ReAct-Kern, 29 Werkzeuge mit gestufter Freigabe, Kontext- und Budgetkontrolle.

-   :material-shield-lock-outline: **[Host-Sicherheitsdesign](host-security.md)**

    ---

    Bewertungsmodell, Kriterien und Schwellen der fünf Prüfungen, Aussperrschutz und Grenzen.

</div>

## Zentrale Architekturentscheidungen

- **Einzel-Server-Monolith**: keine Microservices, Auslieferung als einzelner Prozess und einzelnes Binary
- **SQLite**: eingebettet und wartungsfrei, für das geringe Geschäftsdatenvolumen vollkommen ausreichend
- **Doppelte API**: `/api/v1` für die UI, `/mcp` für Integration, getrennte Authentifizierung und Berechtigungen
- **Reines Go ohne CGO**: plattformübergreifendes Cross-Kompilieren statisch gelinkter Binaries mit einem Klick
