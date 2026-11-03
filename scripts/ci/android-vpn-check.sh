#!/usr/bin/env bash
# Drives MiddleMan's on-device VPN on an Android emulator the way a user
# does (Security → switch → system consent dialog → OK) and collects what
# happened: logcat, screenshots, the app's UI state and the tunnel's.
# Run by .github/workflows/android-vpn-check.yml inside the emulator.
set -u
OUT=${OUT:-vpn-check}
PKG=com.middlelan.app
APK=$(ls dist/android/*.apk | head -1)
mkdir -p "$OUT"
log() { echo "== $*" | tee -a "$OUT/steps.txt"; }

mcp() { # mcp TOOL JSON-ARGS
  curl -s -m 20 -X POST http://127.0.0.1:7600/mcp -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":${2:-{\}}}}"
}
# node_id LABEL [KIND]: the id of the first UI node with this label.
node_id() {
  mcp ui_snapshot "{\"query\":\"$1\"}" | python3 -c '
import sys, json
label, kind = sys.argv[1], (sys.argv[2] if len(sys.argv) > 2 else "")
try:
    d = json.loads(json.loads(sys.stdin.read())["result"]["content"][0]["text"])
except Exception:
    sys.exit(0)
for n in d.get("nodes", []):
    if n.get("label") == label and (not kind or n.get("kind") == kind):
        print(n["id"]); break' "$1" "${2:-}"
}
shot() { adb exec-out screencap -p > "$OUT/$1.png" 2>/dev/null; }
# tap_text TEXT: taps a system UI element (the consent dialog) by text.
tap_text() {
  adb shell uiautomator dump /sdcard/ui.xml >/dev/null 2>&1
  adb pull /sdcard/ui.xml "$OUT/ui-$1.xml" >/dev/null 2>&1
  python3 - "$OUT/ui-$1.xml" "$1" <<'PY' | while read -r x y; do adb shell input tap "$x" "$y"; echo "tapped $x $y"; done
import re, sys
xml = open(sys.argv[1], encoding="utf-8", errors="replace").read()
for m in re.finditer(r'<node [^>]*>', xml):
    n = m.group(0)
    t = re.search(r'text="([^"]*)"', n)
    if t and t.group(1).strip().lower() == sys.argv[2].lower():
        b = re.search(r'bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"', n)
        x1, y1, x2, y2 = map(int, b.groups())
        print((x1 + x2) // 2, (y1 + y2) // 2)
        break
PY
}

log "install $APK"
adb install -r -g "$APK" 2>&1 | tee -a "$OUT/steps.txt"
adb logcat -c
log "launch"
adb shell monkey -p $PKG -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
adb forward tcp:7600 tcp:7600
# Wait until the app's automation server answers (cold starts vary).
for i in $(seq 1 60); do
  if mcp ui_snapshot '{"query":"Security"}' | grep -q '"result"'; then echo "app ready after ${i}x2s"; break; fi
  sleep 2
done
sleep 3
mcp ui_snapshot '{}' > "$OUT/snapshot-start.json"
shot 01-launched

log "open Security"
id=$(node_id "Security" button); echo "security button: $id" | tee -a "$OUT/steps.txt"
[ -n "$id" ] && mcp ui_click "{\"id\":\"$id\"}" >/dev/null
sleep 3
mcp ui_scroll "{\"until\":\"See and filter every app's connections\"}" >/dev/null; sleep 1
sw=$(node_id "See and filter every app's connections" checkbox)
echo "vpn switch: $sw" | tee -a "$OUT/steps.txt"
mcp ui_snapshot '{}' > "$OUT/snapshot-security.json"
shot 02-security

log "turn the VPN on"
[ -n "$sw" ] && mcp ui_click "{\"id\":\"$sw\"}" | tee -a "$OUT/steps.txt"; echo
sleep 4
shot 03-consent
log "accept the consent dialog"
tap_text OK | tee -a "$OUT/steps.txt"
sleep 12
shot 04-after-consent
mcp ui_snapshot '{}' > "$OUT/snapshot-after.json"

log "tunnel state"
{ adb shell ip addr show tun0; adb shell dumpsys connectivity | grep -i -A3 "VPN\|tun0" | head -40; } > "$OUT/tunnel.txt" 2>&1
cat "$OUT/tunnel.txt"

log "traffic through the tunnel"
adb shell "echo -e 'GET / HTTP/1.0\r\nHost: example.com\r\n\r\n' | toybox nc -w 8 example.com 80 | head -3" 2>&1 | tee -a "$OUT/steps.txt"
sleep 6
id=$(node_id "Apps" button); [ -n "$id" ] && mcp ui_click "{\"id\":\"$id\"}" >/dev/null
sleep 3
shot 05-apps
mcp ui_snapshot '{}' > "$OUT/snapshot-apps.json"

# Leaving and coming back: the app must draw again (not stay on the
# splash screen). Checked after Home and after Back, with the VPN on.
alive() { # alive LABEL: does the app draw and answer after returning?
  sleep 8
  shot "return-$1"
  local top; top=$(adb shell dumpsys activity activities | grep -m1 -E "topResumedActivity|mResumedActivity" | tr -d '\r')
  echo "[$1] resumed: $top" | tee -a "$OUT/steps.txt"
  if mcp ui_snapshot '{"query":"Security"}' | grep -q '"result"'; then
    echo "[$1] app answers: yes" | tee -a "$OUT/steps.txt"
  else
    echo "[$1] app answers: NO" | tee -a "$OUT/steps.txt"
  fi
  local frames; frames=$(mcp frame_stats '{}' | grep -o '"frames": *[0-9]*' | head -1)
  echo "[$1] frame stats: $frames" | tee -a "$OUT/steps.txt"
  mcp frame_stats '{"reset":true}' >/dev/null
}
log "leave with Home, come back"
mcp frame_stats '{"reset":true}' >/dev/null
adb shell input keyevent KEYCODE_HOME; sleep 6
adb shell monkey -p $PKG -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
alive home
log "leave with Back, come back"
adb shell input keyevent KEYCODE_BACK; sleep 2
adb shell input keyevent KEYCODE_BACK; sleep 6
adb shell monkey -p $PKG -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
alive back
log "leave with Home for 40 s, come back"
adb shell input keyevent KEYCODE_HOME; sleep 40
adb shell monkey -p $PKG -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
alive long
# Freezes: Android writes the main thread's stack when the app stops
# responding.
adb root >/dev/null 2>&1; sleep 3
adb shell 'ls /data/anr/ 2>/dev/null' | tee -a "$OUT/steps.txt"
adb shell 'cat /data/anr/anr_* 2>/dev/null' > "$OUT/anr.txt"
if [ -s "$OUT/anr.txt" ]; then
  echo "--- ANR main thread"
  awk '/"main"/,/^$/' "$OUT/anr.txt" | head -60
fi

# What the app shows, as text: the job log is where results are read.
for f in security after apps; do
  echo "--- UI text: $f"
  python3 - "$OUT/snapshot-$f.json" <<'PY'
import json, sys
try:
    d = json.loads(json.loads(open(sys.argv[1]).read())["result"]["content"][0]["text"])
except Exception as e:
    print("no snapshot:", e); sys.exit(0)
for n in d.get("nodes", []):
    if n.get("kind") in ("text", "checkbox", "button") and n.get("label"):
        v = n.get("value")
        print(f'{n["kind"]:8} {n["label"][:110]}' + (f'  = {v}' if v not in (None, "") else ""))
PY
done
adb logcat -d -v threadtime > "$OUT/logcat.txt"
pid=$(adb shell pidof $PKG | tr -d '\r')
echo "app pid: ${pid:-not running}" | tee -a "$OUT/steps.txt"
# The app's own lines, plus what the system says about VPNs, foreground
# services and crashes.
{
  [ -n "$pid" ] && grep -E "^\S+ \S+ +$pid " "$OUT/logcat.txt"
  grep -E "Vpn|vpndialogs|ActivityManager.*(middlelan|ForegroundService)|ANR in|Input dispatching|AndroidRuntime|FATAL|middlelan|SplashScreen" "$OUT/logcat.txt"
} | grep -v "avc:  denied" | sort -u -k1,2 > "$OUT/logcat-relevant.txt"
echo "--- relevant logcat ($(wc -l < "$OUT/logcat-relevant.txt") lines)"
tail -150 "$OUT/logcat-relevant.txt"
exit 0
