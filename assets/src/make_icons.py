#!/usr/bin/env python3
"""Generates MiddleMan's icons from SVG with rsvg-convert.

Run from this folder: python3 make_icons.py
Outputs (in ../): icon.png (desktop + store, 1024), android-foreground.png,
android-background.png, android-monochrome.png (adaptive icon layers, 108dp
canvas with the artwork inside the middle 66dp), tray.png and
tray-template.png.
"""
import subprocess

# Neutral graphite with a soft light from above, a hairline gold rim and
# champagne gold artwork: black, gold and ivory.
BG = """
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#1c1c1d"/>
      <stop offset="0.6" stop-color="#111112"/>
      <stop offset="1" stop-color="#09090a"/>
    </linearGradient>
    <radialGradient id="glow" cx="0.5" cy="0.5" r="0.55">
      <stop offset="0" stop-color="#ffffff" stop-opacity="0.06"/>
      <stop offset="1" stop-color="#ffffff" stop-opacity="0"/>
    </radialGradient>
  </defs>
  <rect width="1024" height="1024" fill="url(#bg)"/>
  <rect width="1024" height="1024" fill="url(#glow)"/>
"""

# A hairline gold rim inside the rounded square (desktop icon only).
RIM = '<rect x="10" y="10" width="1004" height="1004" rx="218" fill="none" stroke="#ffffff" stroke-opacity="0.12" stroke-width="5"/>'

EMBLEM_DEFS = """
  <defs>
    <linearGradient id="gold" x1="0" y1="0" x2="0.35" y2="1">
      <stop offset="0" stop-color="#f8e7bd"/>
      <stop offset="0.45" stop-color="#ddb46a"/>
      <stop offset="1" stop-color="#a87b34"/>
    </linearGradient>
    <radialGradient id="pearl" cx="0.38" cy="0.32" r="0.8">
      <stop offset="0" stop-color="#fffdf7"/>
      <stop offset="0.5" stop-color="#f3e6c8"/>
      <stop offset="1" stop-color="#c9a15a"/>
    </radialGradient>
  </defs>
"""

# The man in the middle: a figure between two linked devices, every
# packet passing through him.
HEAD = (512, 392, 96)
BODY = "M340 742 V684 C340 600 404 540 484 532 L512 560 L540 532 C620 540 684 600 684 684 V742 Z"
# The open collar and the tie, cut out of the jacket.
COLLAR = "M466 536 L512 650 L558 536 L540 532 L512 560 L484 532 Z"
TIE = "M512 566 L530 590 L520 604 L534 700 L512 726 L490 700 L504 604 L494 590 Z"
LINKS = ["M150 640 H306", "M738 640 H810"]
NODES = [(150, 640), (874, 640)]


def emblem(color=None):
    """The emblem in gold, or in one flat color for monochrome uses."""
    hx, hy, hr = HEAD
    if color:
        nodes = "".join(f'<circle cx="{x}" cy="{y}" r="44" fill="none" stroke="{color}" stroke-width="16"/><circle cx="{x}" cy="{y}" r="16" fill="{color}"/>' for x, y in NODES)
        links = "".join(f'<path d="{d}" stroke="{color}" stroke-width="16" stroke-linecap="round" stroke-dasharray="2 34"/>' for d in LINKS)
        return f"""
  <mask id="cut"><rect width="1024" height="1024" fill="#fff"/><path d="{COLLAR}" fill="#000"/></mask>
  <circle cx="{hx}" cy="{hy}" r="{hr}" fill="{color}"/>
  <path d="{BODY}" fill="{color}" mask="url(#cut)"/>
  <path d="{TIE}" fill="{color}"/>
  {links}{nodes}
"""
    nodes = "".join(f'<circle cx="{x}" cy="{y}" r="44" fill="#121213" stroke="url(#gold)" stroke-width="12"/><circle cx="{x}" cy="{y}" r="16" fill="url(#pearl)"/>' for x, y in NODES)
    links = "".join(f'<path d="{d}" stroke="#e7c983" stroke-opacity="0.9" stroke-width="14" stroke-linecap="round" stroke-dasharray="2 32"/>' for d in LINKS)
    return f"""
  <circle cx="{hx}" cy="{hy + 10}" r="{hr}" fill="#000" fill-opacity="0.4"/>
  <path d="{BODY}" fill="#000" fill-opacity="0.4" transform="translate(0 10)"/>
  <circle cx="{hx}" cy="{hy}" r="{hr}" fill="url(#gold)"/>
  <path d="{BODY}" fill="url(#gold)"/>
  <path d="{COLLAR}" fill="#141415"/>
  <path d="{TIE}" fill="url(#pearl)"/>
  {links}{nodes}
"""


def svg(body, defs=""):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" width="1024" height="1024">{defs}{body}</svg>'


def scaled(body, s, dy=0):
    """Scales the emblem around the canvas center (and nudges it up so the
    optical center, the middle node, sits in the middle)."""
    t = 512 * (1 - s)
    return f'<g transform="translate({t} {t + dy}) scale({s})">{body}</g>'


def render(name, content, size=1024):
    src = f"{name}.svg"
    with open(src, "w") as f:
        f.write(content)
    subprocess.run(["rsvg-convert", "-w", str(size), "-h", str(size), "-o", f"../{name}.png", src], check=True)


# Desktop and store icon: a full squircle-ish rounded square, transparent
# corners (never white), the emblem large.
render("icon", svg(
    f'<clipPath id="r"><rect width="1024" height="1024" rx="228"/></clipPath>'
    f'<g clip-path="url(#r)">{BG}{scaled(emblem(), 1.0, -24)}{RIM}</g>', EMBLEM_DEFS))

# Android adaptive icon: the background fills the whole 108dp canvas edge
# to edge, so launchers never add a white frame; the foreground keeps the
# emblem inside the 66dp safe zone (0.61 of the canvas).
render("android-background", svg(BG))
render("android-foreground", svg(scaled(emblem(), 0.72, -18), EMBLEM_DEFS))
render("android-monochrome", svg(scaled(emblem("#ffffff"), 0.72, -18)))

# Tray icons: the emblem alone, bold, readable at 16-22px.
render("tray", svg(scaled(emblem(), 1.18, -70), EMBLEM_DEFS), 64)
render("tray-template", svg(scaled(emblem("#000000"), 1.18, -70)), 64)
