#!/usr/bin/env python3
"""Generates MiddleLAN's icons from SVG with rsvg-convert.

Run from this folder: python3 make_icons.py
Outputs (in ../): icon.png (desktop + store, 1024), android-foreground.png,
android-background.png, android-monochrome.png (adaptive icon layers, 108dp
canvas with the artwork inside the middle 66dp), tray.png and
tray-template.png.
"""
import subprocess

BG = """
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#0d1330"/>
      <stop offset="0.55" stop-color="#15123f"/>
      <stop offset="1" stop-color="#241052"/>
    </linearGradient>
    <radialGradient id="glow" cx="0.5" cy="0.56" r="0.5">
      <stop offset="0" stop-color="#2dd4bf" stop-opacity="0.38"/>
      <stop offset="0.45" stop-color="#6366f1" stop-opacity="0.16"/>
      <stop offset="1" stop-color="#6366f1" stop-opacity="0"/>
    </radialGradient>
    <pattern id="grid" width="64" height="64" patternUnits="userSpaceOnUse">
      <path d="M64 0H0V64" fill="none" stroke="#ffffff" stroke-opacity="0.045" stroke-width="2"/>
    </pattern>
  </defs>
  <rect width="1024" height="1024" fill="url(#bg)"/>
  <rect width="1024" height="1024" fill="url(#grid)"/>
  <rect width="1024" height="1024" fill="url(#glow)"/>
"""

EMBLEM_DEFS = """
  <defs>
    <linearGradient id="stroke" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="#22d3ee"/>
      <stop offset="0.5" stop-color="#818cf8"/>
      <stop offset="1" stop-color="#c084fc"/>
    </linearGradient>
    <radialGradient id="core" cx="0.38" cy="0.32" r="0.75">
      <stop offset="0" stop-color="#ffffff"/>
      <stop offset="0.35" stop-color="#a5f3fc"/>
      <stop offset="1" stop-color="#22d3ee"/>
    </radialGradient>
    <radialGradient id="halo" cx="0.5" cy="0.5" r="0.5">
      <stop offset="0" stop-color="#22d3ee" stop-opacity="0.55"/>
      <stop offset="1" stop-color="#22d3ee" stop-opacity="0"/>
    </radialGradient>
  </defs>
"""

# The "M": two uprights meeting in the middle node, the man in the middle
# that every packet passes through.
M_PATH = "M262 730 L330 318 L512 566 L694 318 L762 730"


def emblem(color=None):
    """The emblem in color, or in one flat color for monochrome uses."""
    if color:
        return f"""
  <path d="{M_PATH}" fill="none" stroke="{color}" stroke-width="70" stroke-linecap="round" stroke-linejoin="round"/>
  <circle cx="512" cy="566" r="128" fill="none" stroke="{color}" stroke-width="22"/>
  <circle cx="512" cy="566" r="74" fill="{color}"/>
  <circle cx="262" cy="730" r="54" fill="{color}"/>
  <circle cx="762" cy="730" r="54" fill="{color}"/>
"""
    return f"""
  <circle cx="512" cy="566" r="230" fill="url(#halo)"/>
  <path d="{M_PATH}" fill="none" stroke="#000" stroke-opacity="0.35" stroke-width="74" stroke-linecap="round" stroke-linejoin="round" transform="translate(0 14)"/>
  <path d="{M_PATH}" fill="none" stroke="url(#stroke)" stroke-width="64" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="{M_PATH}" fill="none" stroke="#ffffff" stroke-opacity="0.22" stroke-width="14" stroke-linecap="round" stroke-linejoin="round" transform="translate(0 -14)"/>
  <circle cx="512" cy="566" r="128" fill="#0f1232" fill-opacity="0.7" stroke="url(#stroke)" stroke-width="16"/>
  <circle cx="512" cy="566" r="128" fill="none" stroke="#a5f3fc" stroke-opacity="0.5" stroke-width="4" stroke-dasharray="18 22"/>
  <circle cx="512" cy="566" r="76" fill="url(#core)"/>
  <circle cx="262" cy="730" r="56" fill="#0f1232" stroke="#22d3ee" stroke-width="18"/>
  <circle cx="262" cy="730" r="22" fill="#67e8f9"/>
  <circle cx="762" cy="730" r="56" fill="#0f1232" stroke="#c084fc" stroke-width="18"/>
  <circle cx="762" cy="730" r="22" fill="#e9d5ff"/>
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
    f'<g clip-path="url(#r)">{BG}{scaled(emblem(), 1.0, -24)}</g>', EMBLEM_DEFS))

# Android adaptive icon: the background fills the whole 108dp canvas edge
# to edge, so launchers never add a white frame; the foreground keeps the
# emblem inside the 66dp safe zone (0.61 of the canvas).
render("android-background", svg(BG))
render("android-foreground", svg(scaled(emblem(), 0.72, -18), EMBLEM_DEFS))
render("android-monochrome", svg(scaled(emblem("#ffffff"), 0.72, -18)))

# Tray icons: the emblem alone, bold, readable at 16-22px.
render("tray", svg(scaled(emblem(), 1.18, -70), EMBLEM_DEFS), 64)
render("tray-template", svg(scaled(emblem("#000000"), 1.18, -70)), 64)
