#!/usr/bin/env python3
"""Generates MiddleMan's icons from SVG with rsvg-convert.

Run from this folder: python3 make_icons.py
Outputs (in ../): icon.png (desktop + store, 1024), android-foreground.png,
android-background.png, android-monochrome.png (adaptive icon layers, 108dp
canvas with the artwork inside the middle 66dp), tray.png and
tray-template.png.
"""
import subprocess

# Warm graphite with a soft light from above, a hairline gold rim and
# champagne gold artwork: black, gold and ivory.
BG = """
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#1f1d1a"/>
      <stop offset="0.6" stop-color="#121110"/>
      <stop offset="1" stop-color="#0b0a09"/>
    </linearGradient>
    <radialGradient id="glow" cx="0.5" cy="0.5" r="0.55">
      <stop offset="0" stop-color="#e9c77f" stop-opacity="0.16"/>
      <stop offset="1" stop-color="#e9c77f" stop-opacity="0"/>
    </radialGradient>
  </defs>
  <rect width="1024" height="1024" fill="url(#bg)"/>
  <rect width="1024" height="1024" fill="url(#glow)"/>
"""

# A hairline gold rim inside the rounded square (desktop icon only).
RIM = '<rect x="10" y="10" width="1004" height="1004" rx="218" fill="none" stroke="#e9c77f" stroke-opacity="0.28" stroke-width="5"/>'

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

# The "M": two uprights whose strokes meet in the middle node, the man in
# the middle that every packet passes through.
M_PATH = "M300 742 V292 L512 552 L724 292 V742"
NODE = (512, 552)


def emblem(color=None):
    """The emblem in gold, or in one flat color for monochrome uses."""
    x, y = NODE
    if color:
        return f"""
  <mask id="gap"><rect width="1024" height="1024" fill="#fff"/><circle cx="{x}" cy="{y}" r="104" fill="#000"/></mask>
  <path d="{M_PATH}" fill="none" stroke="{color}" stroke-width="60" stroke-linecap="square" stroke-linejoin="miter" mask="url(#gap)"/>
  <circle cx="{x}" cy="{y}" r="78" fill="none" stroke="{color}" stroke-width="14"/>
  <circle cx="{x}" cy="{y}" r="40" fill="{color}"/>
"""
    return f"""
  <path d="{M_PATH}" fill="none" stroke="#000" stroke-opacity="0.4" stroke-width="58" stroke-linecap="square" stroke-linejoin="miter" transform="translate(0 8)"/>
  <path d="{M_PATH}" fill="none" stroke="url(#gold)" stroke-width="56" stroke-linecap="square" stroke-linejoin="miter"/>
  <circle cx="{x}" cy="{y}" r="104" fill="#121110"/>
  <circle cx="{x}" cy="{y}" r="80" fill="none" stroke="url(#gold)" stroke-width="10"/>
  <circle cx="{x}" cy="{y}" r="44" fill="url(#pearl)"/>
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
