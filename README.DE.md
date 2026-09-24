<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![WebView2](https://img.shields.io/badge/WebView2-0078D4?style=for-the-badge&logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/microsoft-edge/webview2/)
[![License](https://img.shields.io/badge/License-MIT-4B5563?style=for-the-badge)](LICENSE)
[![Sacred](https://img.shields.io/badge/Sacred-Community-8B1A1A?style=for-the-badge&labelColor=1C1410)](https://ancaria.dev)

[Русский](README.md) · [English](README.EN.md)

</div>

# Sacred Mod Loader

Eine einzige EXE, die Mods für Sacred Gold installiert und das Spiel mit ihnen
startet.

Der Launcher trägt den ganzen Mod-Loader in sich. Du legst ihn in den
Spielordner, wählst Mods aus einer Liste und drückst auf Play. Den Rest
erledigt er selbst: Er lädt die Mods herunter, prüft sie und merkt sich deine
Auswahl.

Deine Spieldateien ändert der Launcher nicht. Er legt alles im Spielordner ab,
zum Entfernen löschst du also einfach diesen Ordner.

## Erste Schritte

1. Lade `Sacred Mod Loader.exe` aus den
   [Releases](https://github.com/ancaria-dev/launcher/releases) herunter.
2. Leg die Datei in deinen Sacred-Gold-Ordner, neben `pureHD.exe`,
   `Sacred.exe` oder `Game.exe`.
3. Starte sie. Neben dem Spiel erscheinen die Ordner `launcher` und `mods`.
4. Wähl im Tab Available Mods aus und installier sie.
5. Drück auf Play.

Während du spielst, ist der Launcher ausgeblendet. Sobald du Sacred beendest,
kommt er zurück.

Jede Auswahl speichert er sofort in `launcher\launcher.json`. Deine Mods,
Flags und übrigen Einstellungen bleiben erhalten, auch wenn du das Fenster
ohne zu spielen schließt.

## Mods und Repositories

Der Tab Installed zeigt die Mods im Ordner `mods`. Du kannst jeden davon
einschalten, ausschalten, aktualisieren oder entfernen. Ein JAR, das du selbst
nach `mods` kopierst, taucht hier beim nächsten Start auf.

Der Tab Available zeigt Mods aus Repositories. Das offizielle,
[ancaria-dev/mods](https://github.com/ancaria-dev/mods), ist von Anfang an
dabei. Ein eigenes fügst du über Repositories hinzu: Füg eine HTTPS-Clone-URL
ein, die auf `.git` endet. Im Wurzelverzeichnis muss
`sacred.mods.repository.json` liegen. Git brauchst du dafür nicht.

Vor der Installation prüft der Launcher jeden Mod: die Prüfsumme, die ID im JAR
und die Kompatibilität mit dieser Loader-Version. Fällt ein Mod durch, wird er
nicht installiert.

Ist ein Mod für eine andere Loader-Version gebaut, bleibt er in der Liste,
lässt sich aber nicht einschalten. Eine rote Zeile unter der Beschreibung sagt,
warum. Beheben kann das nur der Autor des Mods.

Ein privates Repository braucht einen Zugriffstoken. Der Launcher verschlüsselt
ihn mit deinem Windows-Konto, auf einem anderen Rechner ist er also nicht
lesbar. Kann Windows den Token nicht verschlüsseln, warnt dich der Launcher,
dass er ihn im Klartext speichert.

## Java

Mods laufen mit Java 21 oder neuer. Ohne Java startet das Spiel trotzdem,
aber es lädt keine Mods. Welches Java der Launcher gefunden hat, steht über dem
Play-Button.

Der Launcher sucht an drei Stellen und nimmt das erste passende Java:

1. `launcher\java` im Spielordner
2. `JAVA_HOME`
3. `PATH`

Passt keins, klick auf Get Java. Wähl eine Distribution und eine Version, und
der Launcher lädt ein JDK nach `launcher\java`. Die Listen kommen aus dem
Katalog von [foojay](https://api.foojay.io/disco/v3.0/), vorausgewählt sind
Oracle und Java 25. PATH, Registry und alles außerhalb des Spielordners
bleiben unberührt.

Veröffentlicht foojay eine Prüfsumme, gleicht der Launcher das Archiv damit ab.
Ein fehlerhafter Download wird gelöscht, und das JDK, das du schon hattest,
läuft weiter.

## Spielversion

Der Loader ist für `pureHD.exe` 2.0.2.118 gebaut, eine Community-Fassung von
Sacred Gold mit HD-Unterstützung und Fehlerkorrekturen. Liegt in deinem Ordner
eine andere Version, zeigt der Launcher eine Warnung mit dem Button Fix?.

Dann öffnen Fix? und Play einen Dialog mit drei Buttons:

- **Download pureHD** lädt die passende Fassung von ancaria.dev, prüft sie und
  legt `pureHD.exe` und `pHD.dll` neben das Spiel. Deine ursprüngliche EXE
  bleibt, wo sie ist. Ältere Kopien der beiden Dateien verschiebt der Launcher
  vorher nach `launcher\purehd-backup`.
- **Run anyway** startet das Spiel, wie es ist. Mods können dann versagen oder
  das Spiel abstürzen lassen, denn eine andere Fassung hat ihren Code an
  anderen Adressen.
- **Cancel** schließt den Dialog.

## Aktualisieren

Erscheint eine neue Version, taucht unter der Versionsnummer im Kopf der Link
**Download & install now** auf. Der Launcher lädt die neue EXE in den
Spielordner, und **Install & Restart** tauscht sie aus und startet neu. Deine
Mods, Einstellungen und das heruntergeladene Java bleiben, wo sie sind.

Ist GitHub nicht erreichbar oder gibt es nichts Neues, zeigt der Launcher gar
nichts an.

## Einstellungen und Fehlersuche

- **Extra flags for Sacred** gibt dem Spiel zusätzliche
  Kommandozeilenargumente mit.
- **Show the console while playing** öffnet eine Konsole mit dem Log des Hosts.
  `Sacred Mod Loader.exe --debug` schaltet sie gleich beim Start ein.
- **Hooks** ist eine eingeklappte Liste der Stellen, an denen sich der Loader
  ins Spiel einhängt. Schalte Stellen ab, um die zu finden, die Ärger macht.
  Ein Klick auf einen Modulnamen schaltet die ganze Gruppe um.

Liegt Sacred Gold unter `Program Files`, starte den Launcher als
Administrator, sonst lässt Windows ihn nicht in den Spielordner schreiben.
Läuft das Spiel selbst als Administrator, braucht der Launcher dieselben
Rechte.

## Bauen

Du brauchst nur Go 1.26:

```powershell
pwsh tools/build.ps1         # dist/Sacred Mod Loader.exe bauen
pwsh tools/install.ps1       # bauen und in den Spielordner kopieren
```

Das Skript lädt Host und JARs aus den Releases, die `dependencies.json`
festlegt. Liegen Checkouts von `protocol` und `coderpack` daneben, baut es sie
stattdessen aus den Quellen. Dafür brauchst du zusätzlich JDK 21, Python 3.11
und Rust 1.98 mit MSVC und LLVM. `-Protocol none -Coderpack none` erzwingt den
Download auch dann, wenn Checkouts da sind. So baut auch die CI.

`tools/install.ps1` liest den Spielpfad aus einer nicht eingecheckten
`.local.settings`:

```
sacred=D:\SteamLibrary\steamapps\common\Sacred Gold
```

Tests:

```powershell
go vet ./...
go test ./... -count=1
```

Wie der Launcher mit den anderen Repositories zusammenhängt, steht in
[CONTRIBUTING](https://github.com/ancaria-dev/.github/blob/master/CONTRIBUTING.DE.md).

## Releases

Die Version steht in `.version`. `pwsh tools/version.ps1` gibt sie aus,
`pwsh tools/version.ps1 0.200.2` setzt eine neue, und `tools/build.ps1 -Bump`
erhöht vor dem Bauen die letzte Stelle.

Auf `master` veröffentlicht die CI `dist/Sacred Mod Loader.exe` und legt den
Tag `v<Version>` an, wenn es ihn noch nicht gibt.

## Lizenz

MIT, siehe [LICENSE](LICENSE).
