# Backup und Wiederherstellung

## Der Grad der Ein-Klick-Fähigkeit ist unterschiedlich

| Aktion | Grad der Ein-Klick-Fähigkeit | Warum |
|---|---|---|
| Sicherung | ✅ Wirklich ein Klick und es läuft, auch zeitgesteuert möglich | Nur lesender Zugriff auf Quelldaten + Schreiben in Zielverzeichnis, bei Fehlschlag einfach erneut versuchen, nicht destruktiv |
| Wiederherstellung | ⚠️ „Ein Klick", aber mit drei Voraussetzungen | Die Wiederherstellung überschreibt Produktionsdaten, daher sind Vorabprüfung, Snapshot vor der Wiederherstellung und automatisches Rollback erforderlich |

Die Wiederherstellung als „ein Klick und überschreiben" umzusetzen, käme einem Löschknopf ohne Rückgängig-Taste für den Benutzer gleich. Daher heißt es in der Oberfläche **Ein-Klick-Wiederherstellung (mit Vorabprüfung und automatischem Rollback)**.

## Was gesichert wird

Drei Objekttypen, eingetragen unter „Sicherungsobjekte“, eines pro Zeile:

| Schreibweise | Bedeutung |
|---|---|
| `/var/www` | Verzeichnis (oder einzelne Datei), gepackt als `tar.gz` |
| `mysql:opsmini` | Datenbank, exportiert über `mysqldump` und anschließend komprimiert |
| `redis:/var/lib/redis/dump.rdb` | Redis-Snapshot-Datei, diese Datei wird direkt gepackt |

Die MySQL-Sicherung verwendet die im Panel bereits konfigurierten Verbindungsinformationen (lokaler Socket), der Parameter ist `--single-transaction --routines --triggers`, um einen konsistenten Snapshot der InnoDB-Tabellen ohne Tabellensperre zu gewährleisten.

**Nicht unterstützt**: PostgreSQL, Docker-Volume, Snapshots auf Blockgeräte-/Dateisystemebene, inkrementelle Sicherungen, Sicherungsverschlüsselung. Diese Punkte werden in zukünftigen Versionen in Betracht gezogen, in dieser Version werden sie nicht zugesagt.

## Wie ein Snapshot aussieht

```
<Datenverzeichnis>/data/backups/web-20261029-020000/
├── manifest.json                 # Metadaten: Objekte, Größe, sha256, Hostname, Version
└── data/
    ├── var_www.tar.gz
    ├── opsmini.sql.gz
    └── redis.rdb.gz
```

Verzeichnisberechtigungen `0700`, Dateien `0600`. In `manifest.json` wird der sha256 jedes Shards aufgezeichnet, der bei der Wiederherstellung einzeln geprüft wird.

Es werden keine „Repository-basierten" Backup-Tools wie restic/borg eingeführt: Sie bieten Deduplizierung, Verschlüsselung und Snapshot-Ketten, sind funktional leistungsstark, bringen aber 15–25MB an Abhängigkeiten mit, und bei der Wiederherstellung muss dasselbe Tool verwendet werden. Derzeit gilt „selbstenthaltende Shards + Manifest": Auf jeder Maschine entpackt, kann die Wiederherstellung erfolgen.

## Sicherungsablauf

```
Alle Objekte validieren (Pfad existiert, Datenbankname nicht leer, redis-Pfad lesbar)
   ↓  Erst alles validieren, dann handeln: Andernfalls schlägt bei 5 Objekten das 2. mit einem Fehler fehl,
      nachdem bereits 4 Archive erzeugt wurden – und es bleibt ein halber Snapshot zurück
Nacheinander ausführen (seriell, um IO-Sättigung zu vermeiden)
   ↓  Sobald ein Objekt abgeschlossen ist, wird es ins Manifest geschrieben; bei einem Fehler entsteht kein „halbes Backup“
Gesamtprüfsumme berechnen → alte Snapshots gemäß Aufbewahrungsrichtlinie bereinigen → Sicherungsverlauf schreiben
```

Die Anzahl der aufzubewahrenden Kopien beträgt standardmäßig 7, `0` bedeutet, alle aufzubewahren. Die Bereinigung löscht die ältesten **gesamten Verzeichnisse**, niemals einzelne Shards (andernfalls wird das Manifest ungültig).

## Wiederherstellungsablauf (drei Kontrollstufen)

**① Vorprüfung**

- Der Snapshot-Name muss zur sekundären Bestätigung eingegeben werden (Schutz vor Fehlauswahl)
- Das übergeordnete Zielverzeichnis existiert und ist beschreibbar
- `mysql`-Client ist verfügbar (bei der Wiederherstellung der Datenbank)
- Snapshot-Dateien sind vollständig, sha256 stimmt mit dem Manifest überein

**② Automatischer Snapshot vor der Wiederherstellung**

Zuerst wird der aktuelle Zustand gesichert (Verzeichnisse durchlaufen dieselbe Packlogik, Datenbanken werden per Dump gesichert); bei einem Fehler kann zu diesem Schritt zurückgekehrt werden.

**③ Ausführung und Rollback**

- Wiederherstellung → Integritätsprüfung
- Jede fehlgeschlagene Prüfung → **automatischer Rollback zum Snapshot aus Schritt ②** → im Event-Center wird ein kritisches Ereignis (critical) erzeugt

Prüfpunkte der Integritätsprüfung: Verzeichnisvergleich von Dateianzahl und stichprobenartige Prüfsummen, MySQL führt `SELECT 1` aus und gleicht die Zeilenzahlen der Schlüsseltabellen ab, Redis `PING` im Vergleich zum Keyspace.

Die Zuverlässigkeit des Rollbacks beruht darauf, dass „rename innerhalb derselben Partition atomar ist": Vor der Wiederherstellung wird das Zielverzeichnis `mv` zu `xxx.pre-restore-<时间戳>`, bei einem Fehler wird `mv` zurückbenannt.

## Panel-Betrieb

**Panel-Einstellungen → Sicherung und Wiederherstellung**:

| Bereich | Inhalt |
|---|---|
| Sicherungsobjekte | Eines pro Zeile, unterstützt Verzeichnisse / `mysql:库名` / `redis:路径` |
| Aufbewahrungsanzahl | Standard 7, 0 bedeutet keine Bereinigung |
| Sofort sichern | Einmal ausführen, nach Abschluss die Snapshot-Liste aktualisieren |
| Snapshot-Liste | Name, Zeit, Größe, Anzahl der Objekte, Vorprüfungsergebnis |
| Wiederherstellung | Nach Eingabe des Snapshot-Namens und Bestätigung ausführen, gibt Vorprüfungsdetails und Ergebnis zurück |
| Sicherungsverlauf | Ergebnis jedes Laufs (inkl. Fehlerursache) |

## Geplante Sicherung

Die Sicherung ist der **vierte Typ der geplanten Aufgabe** (gleichrangig mit Panel-Aufgabe / System-crontab / Benutzer-crontab) und nutzt dieselbe Planung, dasselbe Starten/Stoppen und dieselbe Fehlerwarnung.

Die Konfiguration erfolgt unter **Geplante Aufgabe → Neue Aufgabe → Typ „Sicherungsaufgabe“ wählen**:

| Feld | Beschreibung |
|---|---|
| Sicherungsobjekt | Gleiche Zeilensyntax wie auf der Sicherungsseite |
| Anzahl der Aufbewahrungen | Strategie zum Überschreiben alter Snapshots |
| Ausführungszyklus | cron-Ausdruck, gängige Voreinstellungen (täglich 02:00 usw.) |
| Benachrichtigungskanal | Panel / E-Mail / WeChat Work / DingTalk / Feishu |
| Lokale Kopie behalten | Bei Verwendung eines Remote-Ziels: ob nach erfolgreichem Upload eine lokale Kopie behalten wird |

Es wird **dieselbe Engine** wie bei der manuellen „Sofortsicherung“ verwendet, die Ergebnisse sind vollständig identisch – es gibt keinen Fall, in dem „manuell wiederhergestellt werden kann, geplant aber nicht“.

Bei einem Fehler wird der Aufgabenstatus zurückgeschrieben (`上次状态 / 上次错误 / 连续失败次数`), gleichzeitig wird ein Sicherungsverlaufseintrag erstellt und die integrierte Alarmregel „Anzahl fehlgeschlagener Sicherungen“ ausgelöst.

## Remote-Ziel: Code bereit, in dieser Version nicht aktiviert

In `internal/backup` sind bereits zwei Remote-Speicher implementiert – S3-kompatibel (OSS / MinIO / AWS, eigene SigV4-Signatur) und SFTP –, aber **v1.4.0 wird nicht als Lieferumfang bereitgestellt**:

- Das Panel führt nicht durch die Konfiguration von Remote-Zielen
- Die Dokumentation empfiehlt die Nutzung in der Produktion nicht
- Es wurde keine Abnahme durch Integrationstests auf echter Hardware durchgeführt (nur Unit-Tests und S3-Signaturvektortests)

Wenn eine geografisch getrennte Speicherung erforderlich ist, genügt es, in `config.yaml` ein Ziel hinzuzufügen, um es zu aktivieren – ohne Codeänderung:

```yaml
backup:
  targets:
    - name: oss-bj
      type: s3
      endpoint: oss-cn-beijing.aliyuncs.com
      bucket: my-backup
      region: cn-beijing
      access_key: "..."
      secret_key: "..."
```

Nach der Konfiguration erscheint auf der Backup-Seite die Auswahl „Remote-Ziel“, mit der Snapshots hochgeladen und Remote-Snapshots durchsucht/zurückgeholt werden können. Der Upload erfolgt erst, nachdem der lokale Snapshot vollständig auf die Festplatte geschrieben wurde, sodass ein fehlgeschlagener Upload kein halbes Backup auf dem Remote-Ziel hinterlässt.

**Hinweis**: Diese Kette wurde nicht durch Integrationstests auf echter Hardware verifiziert. Bitte vor dem produktiven Einsatz zunächst in einem Nicht-Produktionsverzeichnis einen vollständigen Roundtrip testen (Upload → Auflistung → Wiederherstellung in ein temporäres Verzeichnis).

## Beziehung zu anderen Modulen

| Modul | Beziehung |
|---|---|
| [Autonomer Agent (Event-Center)]](autonomous-agent.md) | Wiederherstellungsfehler erzeugen critical-Events; Sicherungsfehler können durch Alarmregeln erfasst werden |
| [Änderungserkennung]](change-awareness.md) | Änderungen an der Sicherungskonfigurationsdatei sind selbst ebenfalls eine Änderung |
| [Host-Sicherheit]](security.md) | „Ist eine nutzbare Sicherung vorhanden“ und „Befindet sich die Sicherung an einem externen Standort“ können als Bewertungskriterien für den Sicherheits-Score dienen |

## Bekannte Einschränkungen

- Während der Sicherung großer Verzeichnisse wird kontinuierlich IO belegt; es wird empfohlen, die geschäftliche Hauptverkehrszeit zu meiden (Standard 02:00)
- MySQL wird über den lokalen Socket gesichert, entfernte Datenbankinstanzen werden nicht unterstützt
- Die Wiederherstellung ist „überschreibend": Es werden nur die im Snapshot enthaltenen Objekte wiederhergestellt, neu hinzugekommene Dateien außerhalb des Snapshots werden nicht bereinigt
- Einzelne Snapshots werden nicht verschlüsselt; für Verzeichnisse mit sensiblen Daten stellen Sie bitte die Berechtigungen des Datenverzeichnisses und die Sicherheit des Sicherungsmediums sicher
