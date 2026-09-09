<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![WebView2](https://img.shields.io/badge/WebView2-0078D4?style=for-the-badge&logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/microsoft-edge/webview2/)
[![License](https://img.shields.io/badge/License-MIT-4B5563?style=for-the-badge)](LICENSE)
[![Sacred](https://img.shields.io/badge/Sacred-Community-8B1A1A?style=for-the-badge&labelColor=1C1410)](https://ancaria.dev)

[Русский](README.md) · [English](README.EN.md)

</div>

# Sacred Mod Loader

Eine Datei im Ordner von Sacred Gold genügt. Der Launcher bringt den gesamten
Mod-Loader mit, zeigt installierte und verfügbare Mods an und startet das Spiel
mit deiner Auswahl.

## Für Spieler

1. Lege `Sacred Mod Loader.exe` in den Ordner mit `pureHD.exe`, `Sacred.exe`
   oder `Game.exe`.
2. Starte die Datei. Beim ersten Start legt der Launcher neben dem Spiel die
   Ordner `launcher` und `mods` an. Nach einem Versionswechsel aktualisiert er
   die Dateien in `launcher`.
3. Wähle die gewünschten Mods aus und klicke auf Play.

Während des Spielens blendet sich der Launcher aus. Sobald du das Spiel
beendest, erscheint er wieder. Zuerst startet er `launcher\protocol.exe`, danach
das Spiel. Wenn das Spiel beendet wird, stoppt er auch den Host.

Die EXE enthält den Loader, aber keine Mods. Du kannst Mods über den Reiter
Available installieren oder JAR-Dateien selbst in `mods` ablegen. Eine JAR mit
lesbarer `META-INF/declaration.toml` erscheint beim nächsten Start unter
Installed.

Jede Änderung wird sofort in `launcher\launcher.json` gespeichert. Ausgewählte
Mods, deaktivierte Hooks, zusätzliche Spielparameter, Repositories und die
Konsoleneinstellung bleiben auch erhalten, wenn du das Fenster ohne Play
schließt.

Mods sind in Java geschrieben. Der Loader führt sie in einer eigenen JVM neben
dem Spiel aus. Ohne geeignetes JDK startet Sacred zwar normal, aber kein Mod
wird geladen. Deshalb zeigt der Launcher schon vor dem Klick auf Play an,
welche Java-Installation er gefunden hat.

Er sucht an drei Stellen: zuerst in `launcher\java`, danach unter
`$JAVA_HOME\bin\java.exe` und zuletzt nach `java` im `PATH`. Verwendet wird der
erste Fund mit Java 21 oder neuer. Ist keiner geeignet, nennt die Zeile über
Play trotzdem die zuerst gefundene Java-Version und erklärt, weshalb sie für
den Loader zu alt ist.

Findet der Launcher kein geeignetes JDK, färbt sich diese Zeile rot und daneben
erscheint die Schaltfläche Get Java. Sie öffnet ein kleines Fenster mit zwei
Auswahllisten. Die Angaben stammen von
[foojay](https://api.foojay.io/disco/v3.0/), dem Verzeichnisdienst hinter
javaalmanac.io. Angeboten werden gepflegte, verfügbare OpenJDK-Distributionen
und unterstützte GA-Versionen ab Java 21. Die Listen sind nicht fest im
Launcher hinterlegt und können deshalb auch später veröffentlichte
Distributionen enthalten. Vorausgewählt ist Oracle 25. Je nach gewähltem Build
gibt es nicht für jede Kombination ein Windows-Archiv. Das voreingestellte
Archiv ist ungefähr 200 MB groß. Während des Downloads zeigt die
Fortschrittsanzeige Datenmenge und Prozentwert an. Danach wechselt sie zum
Entpacken.

Das JDK landet ausschließlich in `launcher\java` im Spielordner. Der Launcher
ändert weder den `PATH` noch die Windows-Registrierung und installiert nichts
an anderer Stelle. Wenn du den Ordner von Sacred Gold löschst, wird auch dieses
JDK entfernt. Wenn foojay für den gewählten Build eine SHA-256-Prüfsumme
veröffentlicht, kontrolliert der Launcher den Download damit. Unvollständige
Dateien und Archive mit falscher Prüfsumme werden gelöscht. Entpackt wird in
einen temporären Ordner, sodass ein fehlgeschlagenes Update das vorhandene JDK
nicht beschädigt.

## Mods und Repositories

Installed zeigt die Mods aus `<Spielordner>\mods`. Der Launcher liest ihre
Deskriptoren als normale ZIP-Einträge und führt vor Play keinen Java-Code aus.
Installierte Mods lassen sich aktivieren, deaktivieren, aktualisieren oder
löschen.

Available lädt zunächst
`https://github.com/ancaria-dev/mods.git`. Eine weitere Quelle braucht eine
HTTPS-Clone-URL mit der Endung `.git` und eine
`sacred.mods.repository.json` im Stammverzeichnis. Der Launcher lädt nur den
Index, die benötigten JAR-Dateien und Bilder. Er startet Git nicht und klont
auch nicht das gesamte Repository.

Bevor eine heruntergeladene JAR in `mods` bleibt, prüft der Launcher den
SHA-256-Wert aus dem Index, die Mod-ID in der JAR sowie deren API- und
Loader-Versionsbereiche. Schlägt eine Prüfung fehl, wird die Datei entfernt.

Auch ein Mod, der eine andere API- oder Loader-Version verlangt, bleibt in der
Liste sichtbar. Er ist ausgegraut, lässt sich nicht auswählen und wird nicht
gestartet. Eine rote Meldung nennt den nicht erfüllten Versionsbereich. So ist
sofort erkennbar, dass der Mod für die aktuelle Version neu gebaut oder
angepasst werden muss.

Für private Repositories kann ein Zugriffstoken hinterlegt werden. Funktioniert
Windows DPAPI, verschlüsselt der Launcher das Token für das aktuelle
Windows-Konto, bevor er es in `launcher\launcher.json` speichert. Unter einem
anderen Konto oder auf einem anderen Computer lässt sich der Wert nicht
entschlüsseln. Ist DPAPI nicht verfügbar, warnt die Oberfläche vor der
Speicherung als Klartext.

Alle vom Loader verwendeten Adressen stammen aus `pureHD.exe` 2.0.2.118, dem
HD-Wrapper der Community. Erkennt der Launcher eine andere ausführbare Datei,
Version oder keine Versionsangabe, zeigt er über der Mod-Liste eine Warnung mit
dem gefundenen und dem erwarteten Build an. Play bleibt verfügbar und
ausgewählte Mods werden trotzdem geladen. Da die betreffenden Stellen in einer
anderen EXE an anderen Adressen liegen können, funktionieren einzelne Mods
möglicherweise nicht oder verhalten sich unerwartet.

Die Originaldateien des Spiels bleiben unverändert. Der Loader setzt seine
Hooks ausschließlich im Speicher des laufenden Prozesses. Beim Beenden des
Spiels verschwinden diese Änderungen wieder. Keine Datei der
Originalinstallation wird gepatcht, umbenannt oder ersetzt.
Der Launcher schreibt nur eigene Dateien nach `launcher` und vom Nutzer
verwaltete JAR-Dateien nach `mods`.

Neben der Mod-Liste befinden sich zwei weitere Einstellungen. Der Inhalt des
Feldes Flags wird an Leerraum in einzelne Argumente zerlegt und an das Spiel
weitergereicht. Mit der Checkbox darunter öffnest du während des Spielens eine
Konsole mit dem Protokoll des Hosts. Der Kommandozeilenschalter `--debug` öffnet
die Konsole sofort und aktiviert die Checkbox für diese Sitzung.

Unter der Mod-Liste befindet sich der zunächst eingeklappte Bereich Hooks.
Jeder Eintrag steht für eine Stelle im Spiel, an der der Loader Code einhängt.
Wird ein Hook deaktiviert, bleibt die zugehörige Instruktion unberührt. Für die
Fehlersuche lässt sich mit einem Klick auf einen Modulnamen eine ganze Gruppe
abschalten. Ein weiterer Spielstart zeigt dann, ob der Fehler in diesem Bereich
liegt, ohne dass zwischen den Starts JavaScript geändert werden muss.

Liegt Sacred Gold unter `Program Files`, muss der Launcher als Administrator
ausgeführt werden. Andernfalls darf er nicht in den Spielordner schreiben. Die
gleichen Rechte sind nötig, wenn das Spiel selbst als Administrator läuft.
Sonst kann sich der Frida-Host nicht an den Prozess anhängen.

## Bauen

Für den Build ist Go 1.26 erforderlich. Liegt `protocol` oder `coderpack` neben
diesem Repository, baut das Skript die jeweilige Komponente aus dem Quellcode
und lädt die fehlende Komponente herunter. Die Reihenfolge ist dabei
festgelegt: zuerst coderpack, damit dessen Adresstabelle vorliegt, dann der
Host, dem über `PROTOCOL_AGENT` gesagt wird, welchen Agenten er einbetten soll.
Für einen vollständigen Quell-Build
werden zusätzlich JDK 21, Python 3.11 und Rust 1.98 mit MSVC-Toolchain und LLVM
benötigt. `frida-sys` führt bindgen aus und braucht dafür libclang.

Fehlen die benachbarten Repositories, lädt das Skript `protocol.exe`,
`api.jar` und `zygote.jar` aus den Releases herunter, deren Versionen in
`dependencies.json` festgelegt sind. Derzeit sind das `protocol` und
`coderpack`, beide `0.99.0`. In diesem Fall genügt Go: Der Agent wird nicht
gesondert geladen, denn sein JavaScript steckt samt Adresstabelle minifiziert
in `protocol.exe`. In einer heruntergeladenen `protocol.exe` ist der Agent des
coderpack-Releases enthalten, das deren eigener Build festgelegt hat; lag ein
coderpack-Checkout daneben, sagt das Skript das ausdrücklich. Mit
`-Protocol none -Coderpack none` lässt sich dieser Weg auch bei vorhandenen
Checkouts erzwingen. Die CI verwendet ihn absichtlich bei jedem Push und prüft
damit bei jedem CI-Build, ob sich das Repository eigenständig bauen lässt.

```powershell
pwsh tools/build.ps1         # Payload zusammenstellen, dann die Exe bauen
pwsh tools/build.ps1 -Bump   # dasselbe, vorher die Patch-Nummer in .version erhöhen
pwsh tools/install.ps1       # bauen und das Ergebnis in den Spielordner kopieren
```

Mit `-Protocol`, `-Coderpack` und `-Mappings` lassen sich andere Checkouts
angeben. Beim Quell-Build von `coderpack` übergibt das Skript die gewählte
`mappings.json`, sofern die Datei vorhanden ist. Andernfalls verwendet
`coderpack/tools/addr.py` seine eigene Suchfolge.

Den Pfad zum Spiel liest `tools/install.ps1` aus `.local.settings`. Diese Datei
wird nicht committet, weil Sacred Gold auf jedem Rechner an einer anderen
Stelle liegen kann. Sie enthält eine Zeile:

```
sacred=D:\SteamLibrary\steamapps\common\Sacred Gold
```

Der Build leert `install/payload`, erstellt den Ordner neu und legt dort den
Host, `api.jar`, `zygote.jar` und `VERSION` ab. Mods gehören nicht zum Payload,
Agent-Skripte ebenso wenig: Stattdessen fragt das Skript den abgelegten Host mit
`protocol.exe --hooks` nach seinen Hook-Stellen, denn ein Host, der keine nennt,
hat keinen Agenten in sich. Anschließend bettet `go build` den Payload und die
Windows-Ressourcen in `dist/Sacred Mod Loader.exe` ein.

Die aktuelle Versionsnummer `0.1.20` steht in `.version`. An ihrer Kopie im
Spielordner erkennt der Launcher, ob er seine Dateien erneut entpacken muss.
`-Bump` erhöht vor dem Build den letzten Teil der Versionsnummer.

Die CI läuft für Pull Requests, manuelle Starts und Pushes nach `master`.
Anschließend führt sie diese Prüfungen aus:

```powershell
go vet ./...
go test ./... -count=1
```

Auf `master` veröffentlicht die CI `dist/Sacred Mod Loader.exe` und legt
`v<version>` nur dann an, wenn dieser Tag noch nicht existiert.

| Verzeichnis | Aufgabe |
|---|---|
| `install` | enthält das eingebettete Payload und entpackt es in den Spielordner |
| `mods` | liest `META-INF/declaration.toml` aus jeder JAR-Datei, ohne sie auszuführen, und prüft, ob der Loader den Mod unterstützt |
| `registry` | liest Mod-Repositories und installiert, aktualisiert oder löscht Mods und Bilder |
| `hooks` | fragt den Host im Spielordner nach den Hook-Stellen seines Agenten |
| `conf` | verwaltet die zuletzt gewählten Einstellungen in `launcher/launcher.json` |
| `game` | startet zuerst den Host und danach das Spiel und beendet beide gemeinsam |
| `java` | findet ein geeignetes JDK und lädt bei Bedarf eines herunter |
| `ui` | stellt die eingebettete Oberfläche in einem WebView2-Fenster dar |

## Lizenz

Der Sacred Mod Loader steht unter der MIT-Lizenz. Der vollständige Text befindet
sich in [LICENSE](LICENSE).

Das Projekt begann als Proof of Concept und wird ohne Supportzusage
bereitgestellt. Es sollte zeigen, ob sich Mods in Java für ein altes
Lieblingsspiel überhaupt umsetzen lassen.
