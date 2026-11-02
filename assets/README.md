# Assets

| File | Used for |
| --- | --- |
| `icon.png` | App icon (1024×1024): desktop, installers, window, store listings, Android before 8.0. Rounded square with transparent corners. |
| `android-foreground.png`, `android-background.png`, `android-monochrome.png` | Android adaptive icon layers (`[android]` in `giode.toml`). The background is full-bleed so launchers never add a white frame; the emblem stays inside the 66dp safe zone. |
| `tray.png`, `tray-template.png` | System tray / macOS menu bar icon. |
| `world.bin` | Low-resolution land mask for the connection map (generated from Natural Earth, public domain). |
| `icons/` | UI icons (Lucide, ISC license). |

`src/make_icons.py` regenerates every icon from SVG (needs `rsvg-convert`).
