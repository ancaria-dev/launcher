<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![WebView2](https://img.shields.io/badge/WebView2-0078D4?style=for-the-badge&logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/microsoft-edge/webview2/)
[![License](https://img.shields.io/badge/License-MIT-4B5563?style=for-the-badge)](LICENSE)
[![Sacred](https://img.shields.io/badge/Sacred-Community-8B1A1A?style=for-the-badge&labelColor=1C1410)](https://ancaria.dev)

[Русский](README.md) · [Deutsch](README.DE.md)

</div>

# Sacred Mod Loader

One executable that installs mods for Sacred Gold and starts the game with
them.

The launcher carries the whole mod loader inside it. You drop it in the game
folder, pick mods from a list, and press Play. It handles the rest: it
downloads the mods, checks them, and remembers your choices.

The launcher never changes your game files. It keeps everything inside the game
folder, so removing it means deleting that folder.

## Getting started

1. Download `Sacred Mod Loader.exe` from the
   [releases](https://github.com/ancaria-dev/launcher/releases).
2. Put it in your Sacred Gold folder, next to `pureHD.exe`, `Sacred.exe`, or
   `Game.exe`.
3. Run it. The `launcher` and `mods` folders appear next to the game.
4. Pick mods on the Available tab and install them.
5. Press Play.

The launcher hides while you play and comes back when you quit Sacred.

It saves every choice right away, in `launcher\launcher.json`. Your mods,
flags, and other settings stay put even if you close the window without
playing.

## Mods and repositories

The Installed tab lists the mods in the `mods` folder. You can enable, disable,
update, or remove each one. A jar you copy into `mods` yourself shows up here
the next time you start the launcher.

The Available tab lists mods from repositories. The official one,
[ancaria-dev/mods](https://github.com/ancaria-dev/mods), is there by default.
To add your own, click Repositories and paste an HTTPS clone URL ending in
`.git`. The repository needs `sacred.mods.repository.json` at its root. You
don't need Git installed.

The launcher checks every mod before installing it: the checksum, the ID inside
the jar, and compatibility with this loader version. A mod that fails any check
isn't installed.

A mod built for another loader version stays in the list, but you can't enable
it. A red line under its description says why. Only the mod's author can fix
that.

A private repository needs an access token. The launcher encrypts it with your
Windows account, so no other computer can read it. If Windows can't encrypt the
token, the launcher warns you that it will save it as plain text.

## Java

Mods run on Java 21 or newer. Without it, the game still starts, but no mods
load. The line above Play shows which Java the launcher found.

The launcher looks in three places and takes the first suitable Java:

1. `launcher\java` in the game folder
2. `JAVA_HOME`
3. `PATH`

If none fits, click Get Java. Pick a distribution and a version, and the
launcher downloads a JDK into `launcher\java`. The lists come from the
[foojay](https://api.foojay.io/disco/v3.0/) catalog. Oracle and Java 25 are
selected by default. PATH, the registry, and everything outside the game
folder stay untouched.

When foojay publishes a checksum, the launcher verifies the archive against
it. A bad download is deleted, and the JDK you already had keeps working.

## Game version

The loader targets `pureHD.exe` 2.0.2.118, a community build of Sacred Gold
with HD support and bug fixes. If your folder holds another version, the
launcher shows a warning with a Fix? button.

In that case, both Fix? and Play open a dialog with three buttons:

- **Download pureHD** fetches the right build from ancaria.dev, verifies it,
  and puts `pureHD.exe` and `pHD.dll` next to the game. Your original
  executable stays where it is. The launcher first moves any older copies of
  those two files to `launcher\purehd-backup`.
- **Run anyway** starts the game as it is. Mods may fail or crash the game,
  because another build keeps its code at other addresses.
- **Cancel** closes the dialog.

## Updating

When a new version comes out, a **Download & install now** link appears under
the version number in the header. The launcher downloads the new executable
into the game folder, and **Install & Restart** swaps it in and restarts. Your
mods, settings, and downloaded Java stay where they are.

If GitHub can't be reached or there's nothing new, the launcher shows nothing.

## Settings and troubleshooting

- **Extra flags for Sacred** passes extra command-line arguments to the game.
- **Show the console while playing** opens a console with the host's log.
  Starting `Sacred Mod Loader.exe --debug` turns it on right away.
- **Hooks** is a collapsed list of the points where the loader attaches to the
  game. Switch points off to find the one causing trouble. Click a module name
  to switch its whole group.

If Sacred Gold lives under `Program Files`, run the launcher as Administrator,
or Windows won't let it write to the game folder. If the game itself runs as
Administrator, the launcher needs the same rights.

## Building

Go 1.26 is all you need:

```powershell
pwsh tools/build.ps1         # build dist/Sacred Mod Loader.exe
pwsh tools/install.ps1       # build and copy it into the game folder
```

The script downloads the host and jars from the releases pinned in
`dependencies.json`. If `protocol` and `coderpack` checkouts sit next to this
one, it builds them from source instead. That also takes JDK 21, Python 3.11,
and Rust 1.98 with MSVC and LLVM. `-Protocol none -Coderpack none` forces the
download path even when checkouts are present. CI builds this way.

`tools/install.ps1` reads the game path from an uncommitted `.local.settings`:

```
sacred=D:\SteamLibrary\steamapps\common\Sacred Gold
```

Tests:

```powershell
go vet ./...
go test ./... -count=1
```

[CONTRIBUTING](https://github.com/ancaria-dev/.github/blob/master/CONTRIBUTING.EN.md)
explains how the launcher fits with the other repositories.

## Releases

The version lives in `.version`. `pwsh tools/version.ps1` prints it,
`pwsh tools/version.ps1 0.200.2` sets a new one, and `tools/build.ps1 -Bump`
raises the last number before building.

On `master`, CI publishes `dist/Sacred Mod Loader.exe` and creates the
`v<version>` tag when that tag doesn't exist yet.

## License

MIT, see [LICENSE](LICENSE).
