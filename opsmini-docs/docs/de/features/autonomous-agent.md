# Autonomer Agent (Ereigniszentrale)

Der „KI-Assistent" funktioniert nach dem Prinzip **du fragst, er antwortet**: Du beschreibst ein Phänomen, er untersucht es und liefert ein Ergebnis.
Der „autonome Agent" hingegen **beobachtet selbst**: Der Collector überwacht den Host kontinuierlich im Hintergrund, und sobald er etwas Beachtenswertes entdeckt, startet er eigenständig eine Untersuchung. Anhand der von dir festgelegten Grenzen entscheidet er zwischen „nur protokollieren / erst fragen, dann handeln / automatisch behandeln" und hinterlässt dabei sämtliche Beurteilungen, Grundlagen und Aktionen nachvollziehbar.

Beide nutzen dasselbe Modell, dieselbe Tool-Sammlung und dieselben Guardrails – der Unterschied liegt nur darin, **wer die Initiative ergreift**.

## Ein geschlossener Kreislauf

```
Collector (dauerhaft aktiv, 2s–20s gestaffelt)
   ↓  normalisiert zu Signal (Fingerprint-Deduplizierung, Persistierung in Datenbank)
Aggregation (dieselbe Angelegenheit wird zu einem Incident zusammengefasst, Zähler steigt, Zeilen nicht)
   ↓
Event-Center (die Liste, die du siehst)
   ↓  Abgleich der Strategie nach family + Schweregrad
Strategie-Engine (shadow nur Analyse / assist nur lesend automatisch, Schreiben erfordert Freigabe / auto Whitelist automatisch)
   ↓
Agent mehrstufige Untersuchung (ReAct, Tool-Aufrufe einzeln nachvollziehbar)
   ↓  Aktionen unterliegen Schreibbudget + Abklingzeit + Risikostufe
Verifikation (optionale Assertion: prüfen, ob die Aktion tatsächlich wirksam wurde)
   ↓  bei Verifikationsfehler automatischer Wiederholungsversuch einmal
Aufzeichnung (Event → Lauf → einzelne Aktionen → Bericht → Shadow-Review)
```

## Collector

Sechs Arten von dauerhaft laufenden Collectoren, alle ausschließlich lesend:

| Collector | Was beobachtet wird | Typische Stufe |
|---|---|---|
| Log-Explosion | EWMA-Baseline in Stunden-Buckets + harte Schwelle von 20MB/min, Rückverfolgung des Prozesses, der die Datei hält | high |
| Anmelde-Anomalie | Außerhalb der Arbeitszeit, erstmalig auftretende IP, Erfolg nach Brute-Force, `authorized_keys` gerade geändert | high |
| Web-Verzeichnis-Skript-Ablage | 14 Arten von WebShell-Signaturen, starke Signaturen direkt high | high |
| Verdächtige Prozess-Ableitung | Web-Benutzer startet shell / curl / python, Mining-Prozessnamen | critical |
| Anomale ausgehende Verbindungen | Ausgehende Verbindungen zu unerwarteten Zielen und Ports | high |
| Metrik-Alarm | 24 Regeln zu CPU / Speicher / Datenträger / Last / Dienststatus usw. | medium–high |

Die Erfassungsfrequenz ist nach Kategorie abgestuft (2s–20s), alles erfolgt per Polling oder Dateilesen, es wird kein dauerhaft laufender Agent-Prozess benötigt und die Systemkonfiguration nicht verändert.

## Ereignisaggregation: eine oder mehrere

Wenn dasselbe Objekt wiederholt Probleme verursacht, muss es ein einziges Ereignis mit Zähler sein, nicht ein Bildschirm voller Meldungen. Regeln:

| Szenario | Verhalten |
|---|---|
| Dasselbe Objekt ändert sich wiederholt (Konfigurationsänderung, Dienst-Neustartschleife) | Während der Nichtbehandlung kontinuierlich zusammengefaltet, `次数` inkrementiert |
| Mehrere Quellen im selben Szenario (10 IPs mit Brute-Force) | Zu einem Ereignis zusammengefaltet, Mitgliederliste aufklappbar (Obergrenze 50, Gesamtzahl wird wahrheitsgemäß erfasst) |
| **Metrik-Alarm / Protokoll-Explosion** | **Solange das Ereignis nicht behandelt wurde, wird es kontinuierlich zusammengefaltet** – ein unbehandelter Alarm ist „eine offene Aufgabe", nicht alle 10 Minuten ein neues Ereignis |
| Anmeldeauffälligkeit (erfolgreiche Anmeldung) | Gleiche IP wird innerhalb eines halben Tages zusammengefaltet, tagesübergreifend gilt als neue Beobachtung |
| Nach Ignorieren / Behandeln erneut aufgetreten | **Neues Ereignis anlegen**, nicht mit dem alten Ereignis zusammenführen |

> Vor v1.4 wurden Metrik-Alarme in 10-Minuten-Fenstern aufgeteilt; eine CPU-Regel konnte über Nacht 8 offene Aufgaben erzeugen. Jetzt gilt „Nichtbehandlung bedeutet kontinuierliches Zusammenfalten" – ein Alarm ist eine Zeile.

## Policy-Engine

Eine Policy ist **Daten** (im Panel können Einträge hinzugefügt, geändert und gelöscht werden), kein Code. Jede Policy legt fest: welches Szenario, ab welcher Stufe, mit welchem Modus, mit welchem Budget.

Drei Modi:

| Modus | Leseoperationen | Schreiboperationen | Geeignet für |
|---|---|---|---|
| `shadow` Shadow | Automatisch | **Grundsätzlich abgelehnt**, nur protokolliert, „was beabsichtigt war" | Neue Szenarien zunächst zwei Wochen beobachten |
| `assist` Assistiert | Automatisch | Popup/Warten auf manuelle Freigabe | Standardempfehlung |
| `auto` Automatisch | Automatisch | **Nur risikoarme Aktionen innerhalb der Whitelist** werden automatisch ausgeführt | Bereits ausreichend validierte Szenarien |

Sechs integrierte Policies:

| Policy | Szenario | Standardmodus | Fähigkeiten |
|---|---|---|---|
| `webshell-respond` | WebShell | assist | Forensik → Datei isolieren, Prozess einfrieren, IP temporär sperren (alles rückgängig machbar) |
| `auth-anomaly` | Anmeldeauffälligkeit | assist | Forensik → Quell-IP temporär sperren |
| `proc-anomaly` | Verdächtiger Prozess | assist | Forensik → Einfrieren (kein kill) |
| `log-growth` | Protokoll-Anstieg | shadow | Ermitteln, wer schreibt und warum, nur protokollieren, was beabsichtigt war |
| `net-anomaly` | Auffällige ausgehende Verbindung | shadow (standardmäßig deaktiviert) | Nur-Lese-Untersuchung |
| `change-review` | Verdächtige Änderung | shadow (erzwungen) | Beurteilen, ob diese Änderung bösartig ist, Empfehlung für Rollback/Freigabe geben |

Jede Policy bringt außerdem vier Budgetobergrenzen und eine Abkühlzeit mit: **Obergrenze für Schreibaktionen, maximale Schrittzahl, maximale Laufzeit in Sekunden, maximale Token-Anzahl, Abkühlzeit in Sekunden**. Wird eine davon überschritten, endet der aktuelle Durchlauf sofort und wird wahrheitsgemäß protokolliert.

Wenn die Abkühlzeit aktiv ist, wird im Ereignis ausdrücklich vermerkt: „Policy X wurde abgeglichen, aber diese Policy befindet sich in der Abkühlphase (verbleibend N), diesmal keine automatische Analyse, bitte manuell beurteilen" – es wird nicht stillschweigend übersprungen.

## Angriffskette

Einzelne Ereignisse werden leicht übersehen: Der WebShell landet um 02:10, ein verdächtiger Prozess um 02:14, die externe Verbindung um 02:20 – drei unabhängige Ereignisse, die sich wie drei verschiedene Vorfälle lesen. Die Angriffskette verknüpft sie zu einem einzigen Eindringen.

Fünf Phasen: **Initialer Zugriff → Ausführung/Ablage → Persistenz → Externe Verbindung/Kontrolle → Auswirkung**.

Verknüpfungsgrundlagen (alle erklärbar und diskutierbar):

- Gemeinsame Indikatoren (dieselbe IP, dieselbe Pfadwurzel, dieselbe Domain)
- Zeitliche Nähe (innerhalb von 15 Minuten am stärksten, innerhalb einer Stunde gilt als schwacher Hinweis)
- Phasenfortschritt (neue Phasen erhöhen die Punktzahl, ein Rückschritt in der Phase erfordert stärkere gemeinsame Indikatoren)
- Erst ab einem Gesamtwert ≥ 0.45 wird zusammengeführt

Einige bewusst streng gefasste Regeln:

- **Ein isolierter Brute-Force-Angriff eröffnet keine Kette**. Jeder Host mit öffentlicher IP wird täglich gescannt; fehlgeschlagene Anmeldungen als „Startpunkt einer Angriffskette" zu behandeln, würde diese Funktion sofort unglaubwürdig machen; erst wenn später eine Ablage/externe Verbindung auftritt, wird sie einbezogen
- **Indikator-Alarme und ein explosionsartiger Anstieg von Protokollen werden niemals in die Kette aufgenommen**, sie sind Symptome, keine Phasen
- **Änderungen werden standardmäßig nicht in die Kette aufgenommen**, es sei denn: die Regelüberprüfung stuft sie als verdächtig ein + die Änderung betrifft einen Ort, der einen Neustart übersteht (`authorized_keys`, cron, systemd, sudoers, `ld.so.preload`, PAM, profile usw.) + in der Kette ist bereits eine Ablage oder externe Verbindung aufgetreten
- **Erst ab drei Phasen oder wenn „Ausführung/Ablage + Externe Verbindung/Kontrolle" gleichzeitig auftreten**, wird auf critical hochgestuft und benachrichtigt

> Die Angriffskette erhöht lediglich die Priorität der Aufmerksamkeit. **Sie erweitert nicht die Berechtigungen des Agent**: Budget, Modus und verfügbare Werkzeuge bleiben unverändert, und es werden nicht wegen „Zugehörigkeit zur Angriffskette" mehr Schreiboperationen erlaubt.

## Abwicklungstransaktion: Aktionen müssen entweder rückgängig gemacht werden können oder explizit als nicht rückgängig machbar gekennzeichnet sein

Wenn der Agent Schreiboperationen ausführt, wird jede einzelne im Protokoll der Abwicklungstransaktion erfasst:

- **Umkehrbare Aktionen** (Dateien isolieren, Prozesse einfrieren, IP blockieren, Protokolle abschneiden, Snapshots erstellen) werden bei fehlgeschlagener Abwicklung **in umgekehrter Reihenfolge zurückgerollt**
- **Nicht umkehrbare Aktionen** (Dateien löschen, Befehle ausführen, Konfigurationen ändern) können nicht zurückgerollt werden; im Bericht wird ausdrücklich vermerkt: „N Elemente können nicht rückgängig gemacht werden, manuelle Bestätigung erforderlich“ – statt vorzutäuschen, dass ein Rollback bereits erfolgt sei
- Die Rollback-Ergebnisse werden wahrheitsgemäß in drei Kategorien unterteilt: rückgängig gemacht / nicht rückgängig machbar / Rollback fehlgeschlagen

Auf der Seite mit den Ereignisdetails kann für nicht zurückgerollte Aktionen direkt „Rollback“ angeklickt werden, ohne auf den Agent warten zu müssen.

## Shadow-Review

`shadow`-Modus ausgeführte Maßnahmen erhalten im Nachhinein eine automatische Review: Anhand der **Fakten** wird beurteilt, ob der damalige Plan noch erforderlich war.

| Ergebnis | Bedeutung |
|---|---|
| `would_help` | Die damalige Ausführung war tatsächlich nützlich (Problem besteht weiterhin) |
| `unnecessary` | Es war auch richtig, es damals nicht zu tun (Problem hat sich von selbst gelöst / Fehlalarm) |
| `unknown` | Unzureichende Evidenz, keine Schlussfolgerung |

Dies ist der Schlüssel dazu, den „Shadow-Modus" von „nur beobachten, nicht üben" zu „üben mit Korrektur" zu machen: Mehrere aufeinanderfolgende `unnecessary` zeigen, dass der Strategie-Schwellenwert zu niedrig angesetzt ist und erhöht werden sollte; mehrere aufeinanderfolgende `would_help` zeigen, dass auf assist hochgestuft werden kann.

## Feedback-Unterdrückung

Wenn dieselbe Ereignisgruppe innerhalb von 7 Tagen zweimal manuell ignoriert wird, werden nachfolgende gleichartige Ereignisse **nicht mehr den Agent geweckt** (sie werden weiterhin aufgezeichnet und weiterhin in der Liste angezeigt, nur nicht automatisch analysiert).

Der Grund ist unmittelbar einleuchtend: Einmaliges Ignorieren kann eine einmalige Situation sein, zweimaliges Ignorieren ist ein Konfigurationsproblem. Diese Regel sorgt dafür, dass „Alarme, die dich nicht interessieren" keine Token mehr verbrauchen.

## Eiserne Regeln für Änderungsereignisse

Unabhängig davon, wie die Strategie konfiguriert ist, dürfen die Ereignisse von `family=change` **nur untersucht, aber keine Aktionen ausgeführt werden**:

- Erzwungene Herabstufung auf shadow, Schreibbudget auf null gesetzt, alle Schreibwerkzeuge deaktiviert, Obergrenze für Schritte auf 8

Das ist keine Pedanterie, sondern eine Angriffsfläche: Wenn „das Schreiben einer überwachten Konfigurationsdatei“ den Agenten dazu bringen kann, Aktionen auszuführen, dann gibt das einem Angreifer einen zusätzlichen Remote-Auslösepfad.

## Verwendung des Panels

Die sechs Filter oben auf der Seite „Ereigniszentrum“ zeigen jeweils eine Anzahl:

| Filter | Was wird angezeigt |
|---|---|
| Zu bearbeiten | Bereits eingetreten, noch von niemandem bearbeitet |
| Zu wecken | Auslösebedingung noch nicht erreicht, wartet auf weitere Signale |
| Alle | Keine Filterung |
| Bearbeitet | Untersuchung abgeschlossen oder manuell als gelöst markiert |
| Fehlgeschlagen | Ausführung fehlgeschlagen / Validierung nicht bestanden |
| Ignoriert | Von dir als nicht relevant eingestuft |

In den Ereignisdetails kann man tiefer einsteigen: zugehörige Signale, Agent-Ausführungsprotokolle, die einzelne Aktionskette (Werkzeug, Parameter, Entscheidung, Rohausgabe), Berichte, Angriffsketten-Kontext.

Zwei manuelle Aktionen:

- **Ignorieren**: als nicht relevant markieren, fließt in die Feedback-Unterdrückungsstatistik ein
- **Ich übernehme**: erklären „Diese Sache bearbeite ich selbst“, der Agent handelt nicht mehr automatisch (neue Signale werden weiterhin aufgezeichnet)

## Zeitplanung und Nachvollziehbarkeit

- Bei jedem Lauf wird ein `AgentRun`-Eintrag hinterlegt (Modus, Schrittzahl, Tool-Anzahl, Schreibvorgänge, Dauer, Ergebnis, ob ein Retry erfolgte)
- Berichte werden als einzelne HTML-Datei archiviert und können nach Ereignis geöffnet oder heruntergeladen werden
- Alle Aktionen (Tool-Aufrufe, Genehmigungen, Ablehnungen, Rollbacks) werden im Audit-Protokoll erfasst

## Beziehung zu anderen Modulen

| Modul | Beziehung |
|---|---|
| [Änderungswahrnehmung]](change-awareness.md) | Liefert „wer wann was geändert hat" und ist die Quelle verdächtiger Änderungsereignisse |
| [KI-Assistent]](ai-assistant.md) | Derselbe Satz von Werkzeugen und Schutzmaßnahmen, mit dem Unterschied, dass er von Ihnen manuell ausgelöst wird |
| [Host-Sicherheit]](security.md) | Die Ergebnisse von Baseline-Prüfung und Bedrohungserkennung gelangen als Signale in das Ereigniszentrum |
| [Sicherung und Wiederherstellung]](backup-restore.md) | Fehlgeschlagene Sicherungen erzeugen Ereignisse; fehlgeschlagene Wiederherstellungen erzeugen automatisch critical-Ereignisse |

## Abgrenzung in einem Satz

OpsMini **bietet keinen** unbeaufsichtigten autonomen Root-Agenten. Die Zielform ist „autonome Untersuchung + abgestufte Autorisierung": Der Agent ist für Erkennung, Beweissicherung, Beurteilung und Empfehlung zuständig; destruktive Operationen werden durch den Menschen autorisiert oder durch eine Whitelist eingegrenzt.
