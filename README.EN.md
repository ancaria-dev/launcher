<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![WebView2](https://img.shields.io/badge/WebView2-0078D4?style=for-the-badge&logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/microsoft-edge/webview2/)
[![License](https://img.shields.io/badge/License-MIT-4B5563?style=for-the-badge)](LICENSE)
[![Sacred](https://img.shields.io/badge/Sacred-Community-8B1A1A?style=for-the-badge&labelColor=1C1410)](https://ancaria.dev)

[Русский](README.md) · [Deutsch](README.DE.md)

</div>

# Sacred Mod Loader

Sacred Mod Loader is a single executable that goes in your Sacred Gold folder.
It contains the rest of the loader, shows the mods you have installed, and
starts the game with the mods you select.

## For players

1. Put `Sacred Mod Loader.exe` in the folder that contains `pureHD.exe`,
   `Sacred.exe`, or `Game.exe`.
2. Run it. On the first launch, it creates `launcher` and `mods` folders beside
   the game. It refreshes the loader files whenever the version changes.
3. Select the mods you want, then press Play.

The launcher hides while the game is running and returns after you quit. It
starts `launcher\protocol.exe` before the game, then stops the host after the
game closes.

The executable contains the loader but no mods. Install mods from the Available
tab or put jars in `mods` yourself. A jar with a readable
`META-INF/declaration.toml` appears on the Installed tab the next time you open
the launcher.

Every change is saved to `launcher\launcher.json`. Selected mods, disabled
hooks, game flags, repositories, and the console setting survive even if you
close the window without pressing Play.

Mods are written in Java. The loader runs them in a separate JVM alongside the
game. Sacred can still start without a suitable JDK, but the mods will not load,
so the launcher shows the Java version it found before you press Play.

It checks three places in order: `launcher\java`, `$JAVA_HOME\bin\java.exe`, and
then `java` on `PATH`. It uses the first Java 21 or newer that it finds. If none
is suitable, the line above Play still identifies the first Java it found and
explains why it cannot run the loader.

If the launcher cannot find a suitable JDK, the line turns red and a "Get Java"
button appears beside it. The button opens a panel with distribution and version
lists from
[foojay](https://api.foojay.io/disco/v3.0/), the same discovery service
used by javaalmanac.io. The launcher filters the live catalog instead of keeping
a fixed vendor list. It offers maintained, available OpenJDK builds and
maintained GA releases from Java 21 upward. Some vendor and version combinations
have no Windows archive. Oracle 25 is selected by default. Its archive is about
200 MB, though the exact size depends on the chosen build. The progress display
reports downloaded megabytes and percentage, then switches to extraction.

The JDK is stored in `launcher\java` inside the game folder. The launcher does
not install it elsewhere, change PATH, or write a registry key. Deleting the
Sacred Gold folder also deletes that JDK. When foojay publishes a SHA-256 digest
for the selected build, the launcher checks the archive against it. Failed,
partial, and mismatched downloads are removed. Extraction happens in a staging
directory, so a failed update leaves the existing JDK intact.

## Updating the launcher

The whole loader travels inside one executable, so an update is replacing it
and starting it again. At startup the launcher asks GitHub whether a newer
release exists. If one does, a line appears under the version in the header:
**Version X is available. Download & install now**. If there is none, or GitHub
cannot be reached, nothing appears at all.

The link opens a window that starts the download at once and stays open until
it finishes. The file goes to `launcher\update` inside the game folder, never
to `%TEMP%` and never anywhere outside the game folder. When the download is
done an **Install & Restart** button appears. The launcher puts the new file in
its own place, starts it and closes. The old one is left beside it as
`Sacred Mod Loader.exe.old` and removed on the next start.

Mods, settings and any Java the launcher downloaded stay where they are. If
something goes wrong the window says what, and offers the release page as a
link that opens in your browser.

## Mods and repositories

The Installed tab lists mods from `<game folder>\mods`. The launcher reads each
descriptor as a normal zip entry and runs no Java code before Play. You can
enable, disable, update, or remove an installed mod.

The Available tab starts with
`https://github.com/ancaria-dev/mods.git`. Another source must be an HTTPS Git
clone URL ending in `.git`, with `sacred.mods.repository.json` at its root. The
launcher fetches the index, jars, and images it needs without running Git or
cloning the full repository.

Before a downloaded jar stays in `mods`, the launcher checks the SHA-256 from
the index, the mod ID inside the jar, and the jar's API and loader version
ranges. A file that fails any check is removed.

Mods built for an unsupported loader version remain visible, but they are greyed
out. A red message explains the incompatibility, the checkbox is disabled, and
the mod cannot start. Keeping the entry visible makes the problem easier to
identify. The mod's author must update its declared ranges or rebuild it for
the current API and loader.

Private repositories can use an access token. When Windows DPAPI works, the
launcher encrypts the token for the current Windows account before saving it in
`launcher\launcher.json`. Another account or computer cannot decrypt that
value. If DPAPI is unavailable, the interface warns that the token will be
stored as plain text.

Every address used by the loader comes from `pureHD.exe` 2.0.2.118, the
community HD wrapper. If the executable in your folder has another version or
no version information, a warning above the mod list shows the detected and
expected builds, with a Fix? button. Play first opens a dialog with three
choices: Cancel, Download pureHD, and Run anyway. Run anyway starts the game as
it is, and the selected mods are still passed to the loader. Some mods may do
nothing or behave incorrectly because the expected code may be at different
addresses in another executable.

Download pureHD fetches `sacred.purehd.zip` from ancaria.dev, checks it against
the SHA-256 built into the launcher, and places `pureHD.exe` and `pHD.dll` next
to the game. The original game executable is not touched, and an older copy of
either file is moved to `launcher\purehd-backup` first. Closing the dialog
does not stop the download; Abort does. Once it is installed the warning goes
away and Play starts `pureHD.exe`.

The loader does not patch, rename, or replace the original game files. Its
changes exist only in the running process and disappear when the game closes.
The launcher writes its own files under `launcher` and user-managed jars under
`mods`, plus `pureHD.exe` and `pHD.dll` next to the game when you ask for them.

Two controls appear beside the mod list. The flags field passes its contents
to the game after splitting the text on whitespace. The checkbox below it opens
a console containing the host's log while you play. Starting the launcher with
`--debug` opens the console immediately and checks that box for the current
session.

Below the mod list is a collapsed section called "Hooks." Each chip represents
one place where the loader attaches to the game. Disabling a chip leaves that
instruction untouched. For debugging, click a module name to disable its whole
group, then run the game again to narrow down the problem without editing
JavaScript between launches.

If Sacred Gold is under `Program Files`, run the launcher as Administrator so
it can write to the game folder. Matching elevation is also required when the
game itself runs as Administrator. Otherwise the Frida host cannot attach.

## Building

The launcher requires Go 1.26. If the `protocol` or `coderpack` repository is
checked out beside this one, the script builds that component from source and
downloads any missing component. A full source build also requires JDK 21,
Python 3.11, and Rust 1.98 with the MSVC toolchain and LLVM. The host's
`frida-sys` dependency runs bindgen, which needs libclang.

Without those sibling repositories, the script downloads `protocol.exe`,
`api.jar`, and `zygote.jar` from the releases pinned in `dependencies.json`.
The current pins are `protocol` at `0.101.0` and `coderpack` at `0.102.0`. Only Go is
required for this path. There is no agent to download: the agent's JavaScript
lives inside `protocol.exe`, minified, along with the address table it was built
with. Pass `-Protocol none -Coderpack none` to use downloaded releases even
when sibling checkouts are present. CI always uses this path.

A source build takes the two in dependency order: coderpack first, so its
address table exists, and then the host, which is told through `PROTOCOL_AGENT`
which agent to build itself around. A downloaded `protocol.exe` carries the
agent of the coderpack release its own build pinned, which the script says out
loud when a coderpack checkout was there but unused.

```powershell
pwsh tools/build.ps1         # stage the payload, then build the executable
pwsh tools/build.ps1 -Bump   # the same, raising the patch number in .version first
pwsh tools/install.ps1       # build, then copy the result into the game folder
```

`-Protocol`, `-Coderpack`, and `-Mappings` can point to other checkouts. During
a source build of `coderpack`, the script passes the selected `mappings.json`
when that file exists. Otherwise `coderpack/tools/addr.py` follows its own
lookup chain.

`tools/install.ps1` reads the game folder from `.local.settings`. This local file
is not committed. It contains one line:

```
sacred=D:\SteamLibrary\steamapps\common\Sacred Gold
```

The build clears and recreates `install/payload`, then stages the host,
`api.jar`, `zygote.jar`, and `VERSION`. It does not stage mod jars, and it no
longer stages agent scripts: it asks the staged host for its hook sites with
`protocol.exe --hooks` instead, because a host that names none has no agent in
it. `go build` embeds the payload and Windows resources in
`dist/Sacred Mod Loader.exe`.

The current version is `0.103.1` in `.version`. Its copy in the installed payload
tells the launcher when to unpack updated files. `-Bump` raises the final version
component before building.

CI runs for pull requests, manual dispatches, and pushes to `master`. After the
build it runs:

```powershell
go vet ./...
go test ./... -count=1
```

On `master`, CI publishes `dist/Sacred Mod Loader.exe` and creates
`v<version>` only when that tag does not already exist.

| Directory | What it does |
|---|---|
| `install` | Embeds the payload and writes it to the game folder |
| `mods` | Reads `META-INF/declaration.toml` from each jar without running it, then checks whether the loader supports it |
| `registry` | Reads mod repositories and installs, updates, or removes mods and icons |
| `hooks` | Asks the host in the game folder which attachment sites its agent installs |
| `conf` | Stores the saved choices in `launcher/launcher.json` |
| `game` | Starts the host followed by the game, then stops them together |
| `java` | Finds a suitable JDK and downloads one when needed |
| `ui` | Provides the WebView2 window and embedded page |

## License

Sacred Mod Loader is available under the MIT License. See
[LICENSE](LICENSE).

This project began as a proof of concept for running Java mods in an old
favourite game. Support is not guaranteed.
