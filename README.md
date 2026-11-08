# Open-Kaurava

An open, clean-room engine for **Warhammer 40,000: Dawn of War**, written in Go on top of
Vulkan — no CGO, no game code, no re-exported assets. It boots from an existing Dawn of War
installation, reads the original data files, and renders and drives them with its own code.

The goal is parity with the Dawn of War engine's actual capabilities, and then room to grow in
any direction the original never went.

## What it is

Open-Kaurava is not a mod and not a wrapper around the retail executable. It is an independent
engine that happens to speak the game's native data formats:

- **Archives and modules.** Reads `.module` files, mounts the game's SGA archives, and builds the
  same merged virtual file tree the original engine sees (`DoWDE` by default).
- **Scenarios.** Parses `.sgb` mission files for terrain, props, start positions, and markers.
- **Blueprints.** Reads `.rgd` object blueprints (including Jenkins-hashed field lookups), plus the
  Lua-based `Data/attrib` definitions for units (`ebps`), squads (`sbps`), weapons, and abilities.
- **Interface.** Interprets Relic `.screen` files — the authored, data-only Lua widget trees that
  define the HUD — and draws the original TGA/DDS art through its own atlas and font pipeline.
- **Audio.** Decodes `.fda` sounds (Relic Chunky container with the game's DCT codec), plays music
  from the original playlists, and keeps speech, effects, and music on separate volume channels.
- **Scripting.** Runs game logic in Lua 5.1 through a pure-Go VM, so scenarios can be extended
  without touching engine code.

Everything that makes the game *look* and *sound* like Dawn of War comes from the player's own
installation. Everything that makes it *run* is this repository.

## Why it exists

Dawn of War still has one of the best real-time tactics rule sets ever shipped: squad-level
morale, cover, reinforcement, capture points that feed a resource economy, and an interface that
tells you all of it at a glance. Twenty years on, that design is trapped inside a closed
DirectX 9 engine that no longer builds, no longer scales to modern displays, and cannot be ported
by anyone outside the original studio.

Open-Kaurava takes the position that the *data* is the game, and the engine can be rebuilt around
it. Parity is the near-term objective — a full skirmish loop with the same rules, the same HUD,
and the same feel. Extensibility is the long-term one: once the engine is ours, unit counts,
map sizes, rendering, scripting, and new mechanics stop being fixed by a 2004 binary.

## Inspiration

**Dawn of War's own engine** sets the target. Rules, timings, UI layout, and audio behaviour are
derived from the shipped game rather than invented, which is why the HUD is built from the
original `gamescreen.screen` widget tree instead of a look-alike redrawn by hand.

**[VCMI](https://vcmi.eu/)** proved the method. The Heroes of Might and Magic III engine reimplementation
succeeded by treating the original data files as the source of truth, reimplementing the engine
from scratch, and earning parity feature by feature — starting with what makes the game playable
rather than with what is easiest to code. Open-Kaurava follows the same path: read the real
formats, reproduce the real behaviour, and let the community's data outlive the original binary.

**Freebuff** shapes how the work is done. The engine is developed alongside an agentic coding
environment, which is why the repository carries a set of small probe tools (`tools/`) that dump
game data, HUD geometry, audio streams, and screenshots. Every engine step is grounded in evidence
from the player's own installation instead of guesswork, and interface text is already routed
through a localization layer rather than baked into the code.

## Status

Playable today, in rough order of maturity:

| Area | State |
|---|---|
| Window, Vulkan device, swapchain, resize handling | Working on Windows and Linux (X11/XWayland) |
| Module mounting, SGA archives, scenario list | Working |
| Main menu, scenario picker, player setup | Working |
| 3D scenario view, camera, terrain and props | Working |
| Selection, orders, path movement, formations | Working |
| Construction placement, training, reinforcements, upgrades | Working |
| HUD from original `.screen` data, minimap, command panel | Working |
| Original sounds, music playlists, volume settings | Working |
| Pause menu, restart, settings, English/Russian UI strings | Working |
| Combat resolution, morale, cover, abilities | In progress |
| Save/load, campaign, AI opponents | Planned |
| Mod loading and scripting beyond the current hooks | Planned |

The engine is single-player and offline by design for now; the original's networking is a
separate, much later problem.

## Building and running

Requirements:

- Go 1.27 or newer (`go.mod` pins the exact toolchain).
- A Vulkan-capable driver (NVIDIA, AMD, Intel, or Mesa/llvmpipe).
- Windows 10+ or Linux with X11/XWayland. `CGO_ENABLED=0` is the supported build mode.
- An installed copy of Dawn of War with the module you want to load (the `DoWDE` module is the
  default).
- The sibling [`dow-mod-studio`](../dow-mod-studio-go) checkout: `go.mod` replaces
  `dow-mod-studio` with `../dow-mod-studio-go`, which provides archive, blueprint, and image
  decoding used by the engine.

```sh
go build -o Open-Kaurava.exe .    # or ./Open-Kaurava on Linux
./Open-Kaurava.exe
```

Useful flags:

```
-scene menu|scenarios|players|gameplay   start scene (default menu)
-dow <path>                              Dawn of War installation (auto-detected if omitted)
-scenario <file.sgb>                     scenario to open directly
-script <file.lua>                       game-logic script (on_update/on_input)
-width/-height <px>                      window size (default 1280x720)
-frames <n>                              render n frames and exit
-validation                              enable Vulkan validation layers
-present fifo|mailbox|immediate          force a present mode
-log-events                              print window input events
-font <path.ttf>                         override the UI font
-os-cursor                               keep the system cursor
```

Run the tests with `go test ./...`; the suite covers the Vulkan struct layouts, input gesture
tracking, blueprint parsing, HUD geometry and hit-testing, playlist timing, and pause-menu
controls, and needs no game installation.

## Repository layout

```
main.go              Frame loop: window -> input -> scene update -> record -> present
vulkan/              Pure-Go Vulkan bindings, swapchain, render pass, debug utils
window/              Window creation and input (Win32 and X11 backends), drag tracking
scene/               Scene interface and per-frame context
scenes/              Menu, scenario picker, player setup, gameplay, HUD, pause menu
gfx3d/               Vulkan 3D renderer for scenario geometry, camera, effects feed
ui/                  Text, sprites, atlas packing, TGA/DDS decoding
screenui/            Parser and layout engine for Relic .screen widget trees
entities/            Squads, combat, economy, entity state
attrib/              Readers for Data/attrib blueprints (units, squads, weapons, abilities)
rgdmeta/             Minimal .rgd blueprint reader (hashed field lookup)
mods/                Module discovery, archive mounting, scenario enumeration
audio/, sfx/         FDA decoding, resampling, speech/effects/music playback
lua/                 Lua 5.1 VM and engine hooks for game logic scripts
fx/                  Effect definitions, beams, and particle simulation
settings/, loc/      User settings and the localization layer (English by default, Russian included)
tools/               Data probes, layout checks, screenshot/dump helpers
```

Deeper notes: [`docs/vulkan-window-layer.md`](docs/vulkan-window-layer.md) covers the Vulkan and
window base (in Russian), and [`screenui/overview.md`](screenui/overview.md) documents how the
original HUD screen is parsed and interpreted.

## Roadmap

1. **Core rules parity** — combat resolution, morale, cover, weapons, abilities, and the capture
   economy behaving as they do in the original.
2. **Interface parity** — every HUD element driven by the original `.screen` data, with the
   remaining placeholders replaced by real values.
3. **Content coverage** — all shipped races, scenarios, and modules loading without special cases.
4. **Scripting parity** — SCAR-compatible scenario logic and mod hooks.
5. **Beyond parity** — larger battles, modern rendering, and mechanics the original engine could
   not afford.

## Legal

Open-Kaurava ships no game code, no game assets, and no reimplementation of Relic's source. It is
an independent engine that reads data the user already owns. Dawn of War and all related assets
belong to their respective rights holders. You need your own legitimate copy of the game to run it.
