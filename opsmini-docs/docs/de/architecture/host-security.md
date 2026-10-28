# Host-Sicherheitsdesign

> Dieser Artikel beschreibt, **wie die Funktion „Host-Sicherheit" entworfen und implementiert ist**: was jede Prüfung betrachtet, welche Kriterien gelten, wie die Punkte berechnet werden, wo die Grenzen liegen und warum diese Wahl getroffen wurde.
>
> Zugehöriger Code: `internal/service/{security,baseline,loginsecurity,firewall,fim,threat,securitymonitor}.go`, `internal/api/v1/{security,baseline,fim,threat,firewall}.go`, `internal/router/router.go`.

---

## 1. Designziele und Nicht-Ziele

### Ziele

| Ziel | Konkrete Bedeutung |
| --- | --- |
| **Null Abhängigkeiten** | Kein Agent, keine Kernelmodule, keine Verbindung zu einem zentralen Dienst – das Binary mitbringen und sofort prüfen |
| **Erklärbar** | Jede Prüfung lässt sich aufklappen und zeigt „welche Datei gelesen wurde, welcher Wert gesehen wurde, warum sie als nicht konform bewertet wird, wie man es beheben sollte" |
| **Handlungsfähig** | Nach dem Erkennen eines Problems kann es direkt im Panel behoben werden (Firewall ändern, Check ausführen, Baseline neu erstellen, Bedrohung als behandelt markieren) |
| **Für Menschen und für KI** | Dieselbe Schlussfolgerung wird sowohl als Seite gerendert als auch dem integrierten Agent als schreibgeschütztes Werkzeug übergeben |
| **Keine Rechteüberschreitung** | Leseberechtigungen sind nach der Anmeldung sofort nutzbar; jede systemverändernde Aktion erfordert einen separaten Berechtigungspunkt + root-Identität |

### Nicht-Ziele (Abwägungen, nicht unfertige Arbeit)

- **Kein HIDS-Ersatz**: Keine Kernel-Syscall-Hooks, keine eBPF-Sonden, keine vollständige Traffic-Erkennung. Solche Fähigkeiten erfordern dauerhaft laufende Sammlung mit hohen Rechten und stehen im Konflikt mit der Positionierung „einzelnes Binary, wenige Dutzend MB, beiläufig installierbar".
- **Keine Multi-Host-Lage**: Ein einzelner Host liefert Schlussfolgerungen für einen einzelnen Host. Die Aggregation mehrerer Maschinen ist Aufgabe von OpsMini Server / Proxy.
- **Keine eigene Regelwerk-Engine**: Baseline-Kriterien sind im Code hinterlegt (14 Einträge), kein Hot-Loading von YAML-Regeln. Hot-Loading von Regeln klingt flexibel, macht aber „warum hat diese Maschine 62 Punkte" nicht reproduzierbar. Externe Regeln sind die Entwicklungsrichtung (siehe §16).
- **Kein Ersatz für professionelles Compliance-Scanning**: Keine vollständige Abdeckung und Zertifizierung von MLPS/CIS-Einträgen, nur der Teil, der „im Alltag sichtbar und ausführbar" ist.

---

## 2. Fähigkeitskarte

Die Seite Host-Sicherheit besteht aus sechs Tabs und ist im Wesentlichen **fünf Prüfmodule + eine Übersichtsansicht**:

| Tab | Was wird geprüft | Datenquelle | Auslösung | Ändert das System |
| --- | --- | --- | --- | --- |
| Sicherheitsübersicht | Aggregierte Zusammenfassung: Gesamtpunktzahl, Stufe, Punktzahlen der fünf Module und ein Satz Fazit | In der Datenbank gespeicherte Ergebnisse der übrigen fünf Module | Seite öffnen | Nein |
| Baseline-Prüfung | Aktueller Stand der 14 Host-Härtungsmaßnahmen | `/etc/ssh/sshd_config`、`/etc/passwd`、`/etc/shadow`、`/etc/login.defs`、crontab、lauschende Ports | Manuell auf „Prüfung starten“ klicken | Nein |
| Anmeldesicherheit | Brute-Force, anomalie Anmeldungen, SSH-Härtungsempfehlungen | `/var/log/auth.log` oder `/var/log/secure` | Seite öffnen | Nein |
| Firewall | Aktuelles Backend und Regeln; Ports freigeben/sperren, Start/Stopp | Tatsächlicher Status von `ufw` / `firewall-cmd` / `iptables` | Manuelle Bedienung | **Ja** |
| Dateiintegrität | Ob kritische Dateien geändert/gelöscht wurden | SHA256 von 8 kritischen Systemdateien | Hintergrund alle 60 Sekunden | Nein |
| Bedrohungserkennung | Mining-Prozesse, verdächtige geplante Aufgaben, verdächtige Autostarts | `/proc`、crontab、`/etc/rc.local`、systemd-Dienstliste | Hintergrund alle 5 Minuten + manuell | Nein (Behandlung muss vom Menschen erfolgen) |

Eine klare Designentscheidung: **Nur die Firewall verändert das System**. Die übrigen vier Module sind reine Beobachter; selbst bei Fehlurteilen wird lediglich die Punktzahl schlechter, ohne dass die Maschine zu Schaden kommt.

---

## 3. Architektur: Zwei voneinander unabhängige Pfade

In OpsMini gibt es zwei sicherheitsrelevante Systeme, die ähnlich aussehen, tatsächlich aber zwei unabhängige Pfade sind:

```
                  ┌──────────── Prüfpfad (pull, für Menschen sichtbar) ────────────┐
  /etc、/proc ───▶ │ BaselineService  Login-Analyse  FIM  Bedrohungsscan        │ ──▶ SQLite ──▶ Sicherheitsseite + Bewertung
  Authentifizierungsprotokolle │ SecurityService  Zusammenfassung und Bewertung                        │
                  └──────────────────────────────────────────────────┘

                  ┌──────────── Wahrnehmungspfad (push, für Agent) ──────────┐
  auth.log、Website   │ sentinel Collector (logwatch/authwatch/           │ ──▶ Signal ──▶ Ereignis ──▶ Strategie
  Verzeichnisse、Prozesstabelle     │ filewatch/procwatch/netwatch/metricwatch）      │        ──▶ Agent-Behandlung
                  └──────────────────────────────────────────────────┘
```

**Warum in zwei Pfade aufteilen statt in einen zusammenzuführen?**

| Dimension | Prüfpfad | Wahrnehmungspfad |
| --- | --- | --- |
| Interaktionsmodell | Pull: Du öffnest die Seite / klickst auf Scan | Push: Collector läuft kontinuierlich |
| Ausgabe | Liste + Bewertung (langfristig speicherbarer Zustands-Snapshot) | Signal → Ereignis (zeitgebundener Stream) |
| Konsument | Mensch | Strategie-Engine + AI Agent |
| Bewertungscharakter | „Entspricht der Ist-Zustand den Vorgaben" | „Was ist gerade passiert" |
| Kosten einer Fehlbewertung | Schlechte Bewertung | Mögliche Auslösung automatischer Behandlung |

Eine Zusammenführung würde beide Seiten gegenseitig belasten: Die Prüfung muss jederzeit neu berechnet werden können, die Wahrnehmung muss kontinuierlich inkrementell arbeiten; fügt man einem Pfad eine Zustandsmaschine hinzu, muss der andere sie zwangsläufig mitführen.

**Auch die Kosten müssen klar benannt werden**: Beide Pfade parsen jeweils die Authentifizierungsprotokolle (`service/loginsecurity.go` und `sentinel/authwatch.go` sind zwei verschiedene Regex-Sätze), wodurch ein Risiko der Abweichung bei den Auslegungskriterien besteht. Dies ist eine bekannte technische Schuld, siehe §16.

---

## 4. Sicherheitsbewertungsmodell

Gesamtpunktzahl 100, fünf Module mit festen Gewichtungen:

| Modul | Maximalpunktzahl | Warum diese Gewichtung |
| --- | --- | --- |
| Baseline-Prüfung | 40 | Der einzige Teil, den „du aktiv verbessern kannst", und der den täglichen Betriebszustand am besten widerspiegelt |
| Anmeldesicherheit | 20 | Wenn eine öffentlich erreichbare Maschine Probleme bekommt, beginnt es fast immer hier |
| Firewall | 15 | Vorhanden oder nicht – das entscheidet direkt über die Angriffsfläche |
| Dateiintegrität | 15 | Geänderte kritische Dateien bedeuten meist, dass bereits kompromittiert wurde |
| Bedrohungserkennung | 10 | Die niedrige Gewichtung liegt daran, dass sie am anfälligsten für Fehlurteile ist (siehe §10) |

### Punkteregeln der einzelnen Module (eins zu eins mit dem Code übereinstimmend)

**Baseline (0–40)**: gewichtet nach Gewichtung, nicht nach Anzahl der Einträge.

```
got   = Σ(Gewichtung der pass-Einträge) + Σ(Gewichtung der warn-Einträge × 0.5)
total = Σ(Gewichtung aller Einträge) = 32
Punkte = int(40 × got / total)
```

Dreizustands-Semantik: `pass` zählt als volle Punktzahl, `warn` als halbe Punktzahl, `fail` als 0. Der Grund für die Existenz von warn ist, dass einige Einträge „nicht umgesetzt, aber nicht falsch" sind (z. B. SSH nutzt noch Port 22: das Risiko ist real, aber verbreitet – eine direkte Bewertung mit 0 würde die Aussagekraft der Punktzahl zunichte machen).

**Anmeldesicherheit (0–20)**:

| Situation | Punkte |
| --- | --- |
| Kein Brute-Force festgestellt, 24h keine Fehlschläge | 20 |
| 24h Fehlanmeldungen, aber unter der Brute-Force-Schwelle | 16 (fester Abzug von 4) |
| n verdächtige Brute-Force-Ziele festgestellt | `20 − 5n`, Untergrenze 0 |

**Firewall (0–15)**: aktiviert 15, nicht aktiviert 0. Binäre Bewertung, kein Zwischenzustand.

**Dateiintegrität (0–15)**: `15 − 3n` (n = Anzahl der Änderungen innerhalb von 24h), Untergrenze 0. Ein Abzug von 3 Punkten pro Änderung bedeutet, dass bei 5 geänderten kritischen Dateien die Punktzahl auf null fällt.

**Bedrohungserkennung (0–10)**: Abzug ausgehend von 10, `critical` Abzug 5, `warning` Abzug 2, sonstige Abzug 1, Untergrenze 0.

### Stufenzuordnung

| Punktzahl | Stufe |
| --- | --- |
| ≥ 90 | Ausgezeichnet |
| ≥ 75 | Gut |
| ≥ 60 | Durchschnittlich |
| < 60 | Gefährlich |

### Die Abwägungen dieses Modells

- **Keine Gewichtung nach Asset-Wert**: OpsMini weiß nicht, welche Geschäftsprozesse auf dieser Maschine laufen; einen „Wichtigkeitskoeffizienten" zu raten, würde die Punktzahl nur zu einer obskuren Größe machen.
- **Kein CVSS**: CVSS bewertet die Schwere von Schwachstellen, hier wird der Host-Konfigurations- und Betriebszustand bewertet – das sind zwei verschiedene Dinge.
- **Keine negativen Punkte**: Ein Einzelposten wird bei 0 gestoppt, um zu vermeiden, dass ein Modul die Gesamtpunktzahl zum Einsturz bringt und die Punktzahl dadurch bedeutungslos wird.
- **Punktzahl reproduzierbar**: Gleiche Eingabe, gleicher Code, gleiche Punktzahl. Das ist die Voraussetzung dafür, sie für einen Vergleich im Sinne von „nach der Änderung noch einmal hinschauen" zu nutzen.

---

## 5. Baseline-Prüfung

### Ausführungsmethode

Ein Scan erzeugt einen `ScanID` (Nanosekunden-Zeitstempel), die 14 Ergebnisse werden jeweils in einer Zeile `baseline_results` geschrieben, die Seite zeigt nur den **neuesten Batch** an, historische Batches bleiben in der Datenbank zur Vergleichbarkeit erhalten.

Reine Dateianalyse, es werden keine externen Tools wie `lynis` / `oscap` aufgerufen – diese Tools existieren in minimalen Systemen oft nicht, und ihre Ausgabe ist schwer zu strukturieren.

### Beurteilungsdetails

- `sshd_config` liest gleichzeitig die Hauptdatei und `/etc/ssh/sshd_config.d/*.conf` (moderne Distributionen splitten die Konfiguration in ein drop-in-Verzeichnis auf, nur das Lesen der Hauptdatei würde zu Fehlurteilen führen).
- „Nicht explizit konfiguriert" und „explizit falsch konfiguriert" werden unterschieden: Ersteres ergibt `warn` (Distribution-Standardwert unsicher), Letzteres ergibt `fail`.
- `/etc/shadow`, `/etc/passwd` parsen direkt den Text, ohne Abhängigkeit von der NSS-Konfiguration – um Beurteilungsdrift auf Maschinen mit LDAP/SSSD-Beteiligung zu vermeiden.
- Für Hochrisiko-Ports wird eine feste Liste verwendet: `23 Telnet`, `445 SMB`, `135 RPC`, `1433 MSSQL`, `3389 RDP`.

Die vollständige Liste der 14 Punkte siehe [Anhang A]](#附录-a基线-14-项清单).

### Warum die Gewichtung fest im Code verankert ist

Durch Ändern der Gewichtung ändert sich die Punktzahl. Wäre die Gewichtung konfigurierbar, hätte dieselbe Maschine bei zwei Kunden unterschiedliche Punktzahlen, und die „Baseline-Prüfung" würde ihre Glaubwürdigkeit verlieren. Um die Gewichtung anzupassen, ist ein Code-Review erforderlich.

---

## 6. Anmeldesicherheit

### Datenquelle und Aufwand

Nur das Ende des Authentifizierungsprotokolls wird gelesen: `tail -n 5000`, vorzugsweise `/var/log/auth.log` (Debian-Familie) und dann `/var/log/secure` (RHEL-Familie); ist keines vorhanden, wird ein leerer Bericht statt eines Fehlers zurückgegeben. Das Protokoll kann mehrere hundert MB groß sein; ein vollständiges Einlesen würde den Speicher sprengen.

**Das Statistikfenster muss präzise benannt werden**: Das implementierte Fenster sind „die letzten 5000 Zeilen des Protokolls", kein striktes Zeitfenster. Die Feldnamen folgen `failed_count_24h` / `success_count_24h`, weil bei einer stark angegriffenen Maschine im öffentlichen Netz 5000 Zeilen oft weniger als 24 Stunden abdecken, während bei einer ruhigen Maschine 5000 Zeilen mehrere Tage umfassen können. Diese Abweichung ist ein bekanntes Problem (§15); der Grund, warum keine strikte 24-Stunden-Filterung erfolgt, steht unten.

### Drei Arten von Kriterien

| Kriterium | Regel | Ausgabe |
| --- | --- | --- |
| Brute-Force | Dasselbe Konto schlägt innerhalb des Statistikfensters (einschließlich `Failed password` und `Invalid user`) **kumuliert ≥ 5 Mal** fehl | Kontoname + Anzahl + Liste der beteiligten IPs, absteigend nach Anzahl |
| Top-Fehlerquellen | Fehlschläge nach IP aggregieren, die Top 10 entnehmen | IP + Anzahl |
| Anomale Anmeldung | Bei erfolgreichen Anmeldungen `user == root` | Zeit + Benutzer + IP + Grund „direkte SSH-Anmeldung mit dem root-Konto" |

Die SSH-Härtungsempfehlungen verwenden keinen fest verdrahteten Text, sondern lesen die aktuelle `sshd_config`, um zu beurteilen, ob jede einzelne bereits umgesetzt ist (`PermitRootLogin`, `PasswordAuthentication`, Portänderung, Protocol 2); auf der Seite wird direkt „umgesetzt/nicht umgesetzt" angezeigt.

### Warum keine GeoIP- und UA-Analyse

Die Offline-Umgebung ist eine harte Anforderung. Ein Sicherheits-Panel, das in einem Intranet installiert werden kann, ohne Netzwerkverbindung und ohne Protokolle an externe Dienste zu senden, ist nützlicher als ein Panel, das eine schöne Weltkarte zeichnen kann, aber eine Netzwerkverbindung benötigt. Daher stützt sich die Anomalieerkennung nur auf lokal sichtbare Fakten (Zeit, Konto, IP-Häufigkeit).

### Warum keine strikte 24-Stunden-Filterung

Die Zeitstempel von `syslog` enthalten kein Jahr (`Aug 14 07:00:01`); um nach Zeit zu filtern, muss das Jahr abgeleitet und der Jahreswechsel-Rollover behandelt werden; rund um 0 Uhr nachts, unterschiedliche Zeitzonen von Containern und durch logrotate abgeschnittene Protokolle können die Ableitung fehlerhaft machen. Ein Fehler um ein Jahr bedeutet, dass die gesamte Historie als „gerade eben" gezählt wird.

Statt eines Zeitfensters werden „die letzten N Zeilen" verwendet; der Preis ist eine schwankende Fensterlänge, der Vorteil ist, dass **niemals aufgrund eines Zeitanalysefehlers falsch geurteilt wird**. Wer es wirklich präzise braucht, sollte die Wahrnehmungsschicht gemäß `observed_at` Fensterabfragen in der Datenbank durchführen lassen (diese enthält ohnehin Zeitangaben); dies ist auch eine der Motivationen für die einheitliche Faktenebene in §16.

### Erklärung des Schwellenwerts 5

Weniger als 5 Fehlschläge sind meist manuell falsch eingegebene Passwörter; erst ab 5 Fehlschlägen innerhalb von 24 Stunden von derselben Gruppe von IPs liegt das Merkmal „wird ausprobiert" vor. Dieser Schwellenwert wird auf echten Maschinen im öffentlichen Netz durch massenhaftes Scannen häufig überschritten – genau das ist sein Zweck: **Er dient nicht der präzisen Identifikation, sondern dazu, Ihnen mitzuteilen, dass diese Maschine gerade gescannt wird**.

---

## 7. Firewall

### Automatische Backend-Erkennung

Es wird nicht davon ausgegangen, was auf dem System installiert ist; der tatsächliche Zustand wird gelesen, um das Backend zu bestimmen:

| Backend | Bestimmung | Unterstützte Schreiboperationen |
| --- | --- | --- |
| `ufw` | `ufw status` lesbar | Port freigeben/sperren, aktivieren, deaktivieren, Regeln nach Nummer löschen |
| `firewalld` | `systemctl is-active firewalld` | Port freigeben/sperren (`--permanent` + `--reload`), aktivieren, deaktivieren |
| `iptables` | Regelkette vorhanden | Port freigeben/sperren (`-I INPUT 1`), Regeln nach Nummer löschen; **Deaktivierung im Panel nicht unterstützt** |

Das `iptables`-Backend bietet bewusst keine „Deaktivierung" an: Das Leeren der Regelkette kommt dem Betreiben aller Maschinen ohne Schutz gleich, was zu destruktiv ist; es wird verlangt, dass man selbst auf der Maschine tätig wird.

### Schutz vor Aussperrung (dies ist der wichtigste Punkt dieses Moduls)

**Vor dem Aktivieren der Firewall werden automatisch der eigene Port des Panels und SSH 22 freigegeben**, und danach `enable`.

Die Reihenfolge darf nicht umgekehrt werden. Andernfalls kommt es zum klassischen Unfall: Sobald die Firewall aktiviert ist, werden 8888 und 22 gleichzeitig blockiert, und dem Benutzer bleibt nur noch die Konsole. `panelPort` wird beim Start durch Konfigurationsinjektion bereitgestellt und ist nicht fest auf 8888 verdrahtet.

### Parameterprüfung und Timeout

| Schutzlinie | Vorgehen |
| --- | --- |
| Berechtigung | Alle Schreiboperationen `requireRoot()`, bei Nicht-root wird direkt `ErrNeedRoot` zurückgegeben |
| Port | `int`-Typ + Bereichsprüfung 1–65535, Zeichenketten werden nicht akzeptiert |
| Protokoll | Enumeration `tcp`/`udp` |
| Injektion | Alles läuft über `exec.Command(name, args...)`, nicht über die Shell; Parameter können nicht zu einem zweiten Befehl zusammengesetzt werden |
| Hängenbleiben | Jeder Firewall-Befehl hat ein Timeout von 10 Sekunden (`context.WithTimeout`); bei Fehlschlag wird der Original-stderr zurückgegeben |

### Beziehung zur Agent-Sperrung

Im System gibt es zwei Arten von Sperrungen, die bewusst getrennt verwaltet werden:

| Quelle | Ablageort | Lebenszyklus | Wer räumt auf |
| --- | --- | --- | --- |
| Seitenregeln (manuell konfiguriert) | `ufw`/`firewalld`-Regeltabelle | Langfristig, bis manuell gelöscht | Benutzer |
| Agent-Sperrung (ereignisgesteuert) | `iptables -I INPUT -s <ip> -j DROP` + `blocked_ips`-Tabelle | Mit TTL, heilt bei Ablauf selbst | `SweepBlocks()` hebt automatisch auf |

Grund für die Trennung: Agent-Sperrungen müssen sich selbst heilen können (eine Fehlsperrung muss sich von selbst wieder auflösen können), während manuelle Regeln nicht von der Selbstheilungslogik nebenbei gelöscht werden dürfen. Beide Systeme verwalten sich jeweils selbst und sind auf der Seite getrennt sichtbar.

---

## 8. Dateiintegrität (FIM)

### Überwachungsobjekte

Standardmäßig 8 kritische Systemdateien; beim ersten Start (wenn die Baseline leer ist) wird automatisch eine Baseline erstellt:

```
/etc/passwd  /etc/shadow  /etc/group   /etc/ssh/sshd_config
/etc/crontab /etc/sudoers /etc/hosts   /etc/resolv.conf
```

Änderungen an `/etc/passwd`, `/etc/shadow`, `/etc/sudoers` werden als `critical` protokolliert, die übrigen als `warning`.

### Erkennungsschleife

Alle 60 Sekunden wird für jeden aktivierten Eintrag `SHA256 + size + mtime` berechnet:

- Nicht berechenbar (Datei existiert nicht) → ein Eintrag „Datei gelöscht" wird protokolliert, die Stufe `critical` wird anhand des Pfads bestimmt
- Hash stimmt nicht überein → ein Eintrag „Dateiinhalt geändert" wird protokolliert, mit altem und neuem Hash
- Stimmt überein → es wird nichts unternommen

### Zentrale Designentscheidung: Baseline rückt nach Änderung automatisch vor

Nachdem eine Änderung erkannt und in der Datenbank gespeichert wurde, **wird die Baseline sofort auf den neuen Hash aktualisiert**.

Das ist beabsichtigt: Würde die Baseline nicht vorrücken, würde dieselbe Änderung alle 60 Sekunden gemeldet – 1440 Alarme pro Tag – und niemand würde sie mehr beachten. Der Effekt des Vorrückens ist „eine Änderung, ein Alarm", und **bei der nächsten Änderung wird erneut gemeldet** – wird wirklich wiederholt geändert, bleibt das weiterhin sichtbar.

Der Preis: Finden zwei aufeinanderfolgende Änderungen innerhalb desselben 60-Sekunden-Fensters statt, bleibt nur ein Eintrag übrig. Das ist bei dieser Genauigkeit akzeptabel (für feinere Auflösung wäre inotify nötig, siehe §16).

„Baseline neu erstellen" ist eine manuelle Aktion: Nachdem Sie `sudoers` geändert haben, klicken Sie einmal darauf und teilen dem System mit „der aktuelle Zustand ist korrekt".

### Arbeitsteilung mit filewatch der Wahrnehmungsschicht

|  | Sicherheitsseite FIM | Wahrnehmungsschicht filewatch |
| --- | --- | --- |
| Was wird betrachtet | 8 kritische Systemdateien | Neu hinzugefügte ausführbare Skripte im Website-Stammverzeichnis |
| Kriterium | Hash-Vergleich | Dateiendung + WebShell-Inhaltsmerkmale (`eval($_POST`, `base64_decode` usw.) |
| Zweck | Erkennen, dass „das System manipuliert wurde" | Erkennen, dass „die Website mit einer Shell infiziert wurde" |

Beide überschneiden sich nicht und referenzieren sich nicht gegenseitig.

---

## 9. Bedrohungserkennung

Drei Scanner, eine Runde alle 5 Minuten, beim Start wird zuerst einmal ausgeführt.

| Scanner | Prüft | Kriterium | Stufe |
| --- | --- | --- | --- |
| Prozess | `/proc` Vollständige Prozesse (Name + Kommandozeile) | Treffer auf Mining-Schlüsselwörter (`xmrig`、`minerd`、`kdevtmpfsi`、`kinsing`、`stratum` und weitere insgesamt 11) | critical |
| Prozess | Pfad der ausführbaren Prozessdatei | Befindet sich in `/tmp` oder `/dev/shm` (normale Dienste sollten nicht von hier starten) | warning |
| Geplante Aufgabe | System- + Benutzer-crontab | Befehl enthält `curl`、`wget`、`base64 -d`、`/dev/shm` | warning |
| Autostart-Eintrag | `/etc/rc.local` Nicht-Kommentarzeilen | Enthält `curl`、`wget`、`/tmp` | warning |
| Autostart-Eintrag | Namen der aktivierten systemd-Dienste | Enthält `miner`、`xmrig`、`kdevtmpfs` | critical |

### Deduplizierung und Lebenszyklus

Deduplizierung nach `kind + name`: Bei einem Treffer auf einen bereits vorhandenen Eintrag werden nur `last_seen` und die Details aktualisiert, es wird kein neuer Eintrag angelegt. Status gibt es nur `active` / `resolved`, **nur ein Mensch kann ihn als gelöst markieren** – bevor der Prozess beendet wird, sieht der Scanner ihn alle 5 Minuten erneut, und genau das wollen wir (eine echte Bedrohung sollte nicht von selbst verschwinden).

### Eine ehrliche Bewertung dieser Erkennung

Eine Schlüsselwort-Blacklist **lässt sich durch bloßes Umbenennen umgehen**, das ist ein offenes Geheimnis. Ihre Einordnung ist daher nicht „fängt jedes Mining", sondern:

1. **Faule Angriffe** abfangen – die überwiegende Mehrheit der Botnetze setzt fertige Binaries ein, ohne den Namen zu ändern;
2. In Kombination mit **Verhaltensmerkmalen** wie „ausführbar im temporären Verzeichnis" den umbenannten Teil abdecken;
3. Der Rest wird durch Prozessverhalten und Anmeldeverhalten-Signale der Wahrnehmungsschicht ergänzt.

Kein Anspruch, töten zu können – erkennen und melden, die Behandlung überlässt man dem Menschen (oder einem vom Menschen autorisierten Agent).

---

## 10. Datenmodell

| Tabelle | Eine Zeile repräsentiert | Schlüsselfelder |
| --- | --- | --- |
| `baseline_results` | Ein Ergebnis eines Scans | `scan_id` (Batch), `category`, `item_key`, `status`](pass/fail/warn), `detail`, `suggestion`, `weight` |
| `fim_baselines` | Die aktuelle Baseline einer überwachten Datei | `path`](unique), `sha256`, `size`, `mod_time`, `enabled` |
| `fim_changes` | Ein Dateiänderungsereignis | `path`, `old_sha256`, `new_sha256`, `level`](warning/critical), `message`, `created_at` |
| `threat_findings` | Ein Bedrohungseintrag | `kind`](process/cron/startup), `name`, `detail`, `severity`, `status`, `first_seen`, `last_seen`, `resolved_at` |

Design-Hinweise:

- Baseline-Ergebnisse werden nach `scan_id` in Batches aufgeteilt, **historische Daten werden niemals überschrieben**: Ein Vergleich „letzte Woche und diese Woche" ist abfragbar.
- `fim_baselines` und `fim_changes` sind in getrennten Tabellen: Eines ist ein Zustand, das andere ein Verlauf; eine gemeinsame Tabelle würde „aktuelle Baseline" und „Änderungsverlauf" gegenseitig verunreinigen.
- `threat_findings` enthält `first_seen/last_seen`: Auf einen Blick erkennbar, ob etwas gerade erst aufgetreten ist oder seit drei Tagen unbehandelt besteht.
- Es gibt keine separate Sicherheitsdatenbank-Datei; alles liegt in `opsmini.db` und wird mit dem Backup mitgeführt (konsistent mit den übrigen Daten des Panels).
- Tabellennamen beachten: Das Sicherheits-Baseline-Ergebnis ist `baseline_results`, die „Lern-Baseline" (EWMA-Erwartungswert) der Wahrnehmungsschicht ist `baselines` – beide sind unabhängig voneinander, nicht verwechseln.

---

## 11. API und Berechtigungen

Alle Schnittstellen befinden sich unter `/api/v1` und sind nach JWT-Authentifizierung zugänglich; Schreibvorgänge erfordern zusätzlich Berechtigungspunkte.

| Methode | Pfad | Fähigkeit | Berechtigungspunkt |
| --- | --- | --- | --- |
| GET | `/security/overview` | Gesamtpunktzahl, Stufe, Zusammenfassung der fünf Module | Anmeldung ausreichend |
| GET | `/security/baseline/latest` | Ergebnisse der neuesten Prüfungsrunde | Anmeldung ausreichend |
| POST | `/security/baseline/scan` | Eine Prüfung ausführen | `security.scan` |
| GET | `/security/login` | Analyse der Anmeldesicherheit | Anmeldung ausreichend |
| GET | `/security/login/ssh` | SSH-Härtungsempfehlungen und aktueller Stand | Anmeldung ausreichend |
| GET | `/security/firewall` | Backend, Start-/Stopp-Status, Regelliste | Anmeldung ausreichend |
| POST | `/security/firewall/port` | Port freigeben | `security.firewall` |
| DELETE | `/security/firewall/port` | Port sperren | `security.firewall` |
| POST | `/security/firewall/enable` | Firewall aktivieren | `security.firewall` |
| POST | `/security/firewall/disable` | Firewall deaktivieren | `security.firewall` |
| DELETE | `/security/firewall/rule/:ref` | Regel löschen | `security.firewall` |
| GET | `/security/fim/baselines` | Baseline-Liste | Anmeldung ausreichend |
| POST | `/security/fim/rebuild` | Baseline neu erstellen | `security.fim` |
| DELETE | `/security/fim/baselines/:id` | Ein Überwachungselement entfernen | `security.fim` |
| GET | `/security/fim/events` | Änderungsprotokoll | Anmeldung ausreichend |
| GET | `/security/threats` | Bedrohungsliste | Anmeldung ausreichend |
| POST | `/security/threats/scan` | Sofort scannen | `security.threat` |
| POST | `/security/threats/:id/resolve` | Als behandelt markieren | `security.threat` |

**Lese-Schnittstellen haben grundsätzlich keine Berechtigungspunkte**: Um Probleme auf dem eigenen Rechner zu erkennen, ist keine Autorisierung erforderlich; erst Änderungen am System benötigen sie. Berechtigungspunkte werden von RBAC-Rollen gehalten, eine gewöhnliche Nur-Lese-Rolle kann nur ansehen.

---

## 12. Sicherheitsgrenzen des Panels selbst

Das Sicherheitsmodul ist selbst eine Angriffsfläche, daher:

| Risiko | Behandlung |
| --- | --- |
| Command-Injection | Alle externen Befehle `exec.Command` + Argument-Array, niemals Shell-Verkettung; Ports sind `int`, Protokolle sind Enumerationen |
| Rechteausweitung | Schreiboperationen zuerst `requireRoot()`, Nicht-root wird direkt abgelehnt, kein „Best-Effort" |
| Lange Blockierung | Externe Befehle mit 10 Sekunden Timeout |
| Selbstaussperrung | Vor Aktivierung der Firewall zuerst Panel-Port und 22 freigeben |
| Privatsphäre | Nur Lese-Protokolle für Login-Ereignisse, keine Passwörter, kein vollständiger Befehlstext (nur zur Entscheidung benötigte Felder gelangen in die Datenbank) |
| Unbefugte Aufrufe | Jede Schreib-Schnittstelle hat einen eigenen Berechtigungspunkt, kein Ausblenden von Buttons im Frontend zur „Kontrolle" |

---

## 13. Leistung und Ressourcenaufwand

| Aktion | Aufwand | Beschreibung |
| --- | --- | --- |
| Baseline-Prüfung | 14 kleine Dateilesevorgänge + eine Port-Tabelle, Millisekundenbereich | Läuft nur, wenn ein Benutzer klickt |
| Anmeldeanalyse | `tail -5000` einmal, einmalig < 50ms | Läuft beim Öffnen der Seite, kein Caching |
| FIM-Prüfung | 8 Dateien SHA256, alle 60 Sekunden | Ausschließlich kleine Dateien im KB-Bereich, CPU vernachlässigbar |
| Bedrohungsscan | Durchlaufen von `/proc` Hunderten von Prozessen + crontab + systemctl einmal, alle 5 Minuten | Die einzige „schwere" Aktion, dennoch im Bereich von hundert Millisekunden |
| Dauerhaft belegter Speicher | Ohne zusätzlichen Cache, alles über dieselbe SQLite-Verbindung | Kein eigener Prozess, keine Hintergrundwarteschlange |

Kein dauerhaft laufender Sampler, kein zusätzlicher goroutine-Pool: FIM und Bedrohungsscan teilen sich eine Schleife (`SecurityMonitor`), ein ticker verwaltet eine Aufgabe.

---

## 14. Beziehung zu autonomen Agenten

Drei Schnittstellen, alle **einseitig faktenliefernd**, es gibt keine Kopplung wie „Sicherheitsmodul kommandiert Agent":

| Richtung | Form | Beschreibung |
| --- | --- | --- |
| Sicherheit → AI | Nur-Lese-Tool `security_status` | Der Agent kann „Bewertung + Zusammenfassung der fünf Module" lesen, um zu beantworten „Ist diese Maschine sicher?", die Tools sind `RiskReadonly` + `security.view` |
| Sicherheit → AI (Schreiben) | Tool `firewall_rule` | Das Ändern der Firewall ist eine Schreiboperation, läuft über L1/L2-Klassifizierung, erfordert Bestätigung |
| Wahrnehmungsschicht → Agent | Signal → Ereignis → Strategie | Wird eigenständig von sentinel verantwortet, teilt keinen Zustand mit dem Sicherheitsmodul dieses Abschnitts |

**Was derzeit nicht gemacht wird (bewusst so entschieden)**: Ergebnisse der Bedrohungserkennung und FIM-Änderungen gelangen nicht in den Event-Bus. Der Grund ist, dass die Fehlalarmrate dieser beiden Ergebnisarten höher ist als die der Signale der Wahrnehmungsschicht (Keyword-Matching und automatisches Vorrücken der Baseline erzeugen Rauschen), und es zu riskant ist, sie direkt an automatische Behandlungsstrategien zu verfüttern. Sobald Messdaten zur Fehlalarmrate vorliegen, werden sie angebunden, siehe §16.

---

## 15. Bekannte Einschränkungen

| Einschränkung | Auswirkung | Anmerkung |
| --- | --- | --- |
| Keine Erkennung auf Kernel-Ebene | Rootkits, versteckte Prozesse (`/proc` wird gehookt) sind nicht sichtbar | Durch die Positionierung bedingt, nicht geplant |
| FIM deckt nur 8 Dateien ab | Benutzerdefinierte Pfade, Verzeichnisrekursion, Überwachung neu hinzugefügter Dateien fehlen | Derzeit wird das Website-Verzeichnis über filewatch der Wahrnehmungsschicht abgedeckt |
| Bedrohungserkennung basiert auf Schlüsselwörtern | Umbenennen genügt zur Umgehung | Siehe die ehrliche Bewertung in §9 |
| Baseline mit 14 festen Prüfpunkten | Kann nicht alle Einträge von Multi-Level Protection/CIS abdecken | Die Entwicklungsrichtung ist die Auslagerung von Regeln |
| Bewertung berücksichtigt keine geschäftliche Wichtigkeit | Testmaschine und Kern-Datenbank-Server erhalten dieselbe Punktzahl | Nicht geplant, um Willkür zu vermeiden |
| Abhängigkeit vom Format der Authentifizierungsprotokolle | Maschinen ohne rsyslog/journald-Textformat können möglicherweise nicht gelesen werden | Kompatibilität wird durch `/var/log/secure` abgesichert |
| Das Fenster der Anmeldestatistik ist gleitend | `tail -5000` Zeilen, nicht strikt 24 Stunden (Feldname bleibt 24h) | Siehe §6 „Warum keine strikte 24-Stunden-Filterung implementiert wurde“ |
| Zwei Protokoll-Parser | Die Definitionen von Sicherheitsseite und Wahrnehmungsschicht können auseinanderdriften | Bekannte technische Schuld |
| Kein Berichtsexport | Nur Ansicht auf der Seite, kein PDF/HTML-Compliance-Bericht möglich | Entwicklungsrichtung |

---

## 16. Entwicklungsrichtung

Nach Priorität:

1. **FIM konfigurierbar**: Unterstützung benutzerdefinierter Pfade, rekursiver Verzeichnisse, Whitelists (häufig wechselnde Dateien unter `/etc/` zunächst ausschließen) sowie ereignisgesteuerter inotify-Alternative zum 60-Sekunden-Polling.
2. **Baseline-Regeln externalisieren**: 14 Einträge → YAML-Regelpaket, Unterstützung für den Import von CIS- / Level-2-Klassifizierungseinträgen, gleichzeitig wird "welches Regelset aktuell verwendet wird" im Ergebnis festgehalten, um die Reproduzierbarkeit der Punktzahl zu gewährleisten.
3. **Einheitliche Log-Faktenebene**: `service/loginsecurity.go` und `sentinel/authwatch.go` teilen sich einen Parser, um Abweichungen in der Auslegung zu beseitigen.
4. **Bedrohungserkennung erweitern**: Anbindung von `ausearch` (auditd-Audit-Protokoll) und optionalen YARA-Regeln, Upgrade von Schlüsselwörtern auf zweigleisige Verhaltens- und Signaturerkennung.
5. **Sicherheitsergebnisse in den Event-Bus**: Nachdem die Fehlalarmrate quantifizierbar ist, werden Bedrohungen der Stufe `critical` und FIM-Änderungen der Stufe `critical` an die Policy-Engine angebunden, damit der Agent autonom handeln kann (z. B. Erkennung eines abgelegten WebShell → Isolierung → Bericht).
6. **Compliance-Bericht exportieren**: Ein-Klick-Export eines HTML-Berichts als Einzeldatei (unter Verwendung derselben Rendering-Logik wie beim Bericht der geplanten Aufgabe), mit Punktzahl, Einzelergebnissen und Verbesserungsempfehlungen.

---

## Anhang A: Baseline-Liste mit 14 Punkten

| Kategorie | Punkt | Gewicht | Bewertung |
| --- | --- | --- | --- |
| ssh | SSH deaktiviert Root-Passwort-Anmeldung | 3 | `PermitRootLogin` ist `no`/`prohibit-password` → pass; nicht konfiguriert → warn; sonst → fail |
| ssh | SSH deaktiviert Passwort-Anmeldung | 3 | `PasswordAuthentication no` → pass, sonst warn |
| ssh | SSH ändert Standardport 22 | 1 | nicht 22 → pass, sonst warn |
| ssh | SSH-Protokollversion ist 2 | 1 | nicht konfiguriert oder `2` → pass, sonst fail |
| password | Keine Konten mit leerem Passwort | 3 | `/etc/shadow` Passwortfeld leer → fail, Kontonamen auflisten |
| password | Maximale Passwortgültigkeit ≤ 90 Tage | 2 | `PASS_MAX_DAYS` ≤ 90 → pass, sonst warn |
| account | Keine überflüssigen UID=0-Konten | 3 | außer root UID=0 vorhanden → fail, Kontonamen auflisten |
| account | Keine anmeldefähigen Konten ohne Passwort | 2 | Login-Shell vorhanden und shadow leer/gesperrt → fail |
| file_perm | `/etc/shadow` Berechtigungssicherheit | 2 | zu weitreichende Berechtigungen → warn/fail |
| file_perm | `/etc/passwd` Berechtigungssicherheit | 2 | wie oben |
| file_perm | `/etc/ssh/sshd_config` Berechtigungssicherheit | 1 | wie oben |
| firewall | Firewall aktiviert | 3 | ufw/firewalld/iptables wirksam erkannt → pass, sonst fail |
| firewall | Keine kritischen Ports im Listening-Zustand | 3 | 23/445/135/1433/3389 im Listening-Zustand → warn, Ports auflisten |
| cron | Keine verdächtigen geplanten Aufgaben | 3 | crontab-Befehl enthält `curl`/`wget`/`base64 -d`/`/tmp/`/`/dev/shm` → warn (max. 5 Einträge auflisten) |

Gewichtssumme 32. Gemäß obiger Formel auf 40 Punkte umgerechnet.

---

## Anhang B: Bewertungsbeispiel

Ein typischer Ubuntu-Rechner im öffentlichen Netz:

| Modul | Messung | Punktzahl |
| --- | --- | --- |
| Baseline | 12 von 14 Prüfungen bestanden, 2 Warnungen (SSH auf Port 22, `PASS_MAX_DAYS` nicht gesetzt) | got = 32 − 1×0.5 − 2×0.5 = 30.5 → `int(40×30.5/32)` = **38** |
| Anmeldung | 2000 Fehlversuche innerhalb von 24h, 3 Brute-Force-Ziele getroffen | `20 − 5×3` = **5** |
| Firewall | ufw aktiviert | **15** |
| Dateiintegrität | 1 Änderung innerhalb von 24h (`/etc/hosts`) | `15 − 3` = **12** |
| Bedrohungserkennung | 1 Warnung (ausführbarer Prozess unter `/tmp`) | `10 − 2` = **8** |
| **Gesamtpunktzahl** |  | **78 → Gut** |

Diese Punktzahl ist in der Interpretation belastbar: Die Konfiguration ist grundsätzlich vernünftig (Baseline 38), aber es findet gerade ein Brute-Force-Angriff statt (Anmeldung 5); die Abzüge stammen hauptsächlich von echten Angriffen und nicht von Fehlalarmen. Der Behebungspfad ist ebenfalls klar – Quellbeschränkung in der Firewall aktivieren, fail2ban einsetzen oder per Agent sperren, dann kommt die Anmelde-Punktzahl zurück.
