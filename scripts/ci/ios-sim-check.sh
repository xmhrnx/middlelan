#!/usr/bin/env bash
# Runs MiddleMan in an iPhone simulator and checks it through giode's
# MCP automation server (the simulator shares the Mac's network, so the
# server is on 127.0.0.1:7600). Fails when a check fails.
set -u
OUT=ios-check
mkdir -p "$OUT"
BUNDLE=com.middlelan.app
failed=0
log() { echo "== $*" | tee -a "$OUT/steps.txt"; }
fail() { echo "FAILED: $*" | tee -a "$OUT/steps.txt"; failed=1; }

mcp() { # mcp TOOL JSON-ARGS
  curl -s -m 20 -X POST http://127.0.0.1:7600/mcp -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":${2:-{\}}}}"
}
# labels: every UI node label in the current snapshot, one per line.
labels() {
  mcp ui_snapshot '{}' | python3 -c '
import sys, json
try:
    d = json.loads(json.loads(sys.stdin.read())["result"]["content"][0]["text"])
except Exception:
    sys.exit(0)
for n in d.get("nodes", []):
    if n.get("label"):
        print(n.get("kind", ""), n["label"])'
}
node_id() { # node_id LABEL [KIND]
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
click() { local id; id=$(node_id "$1" "${2:-}"); [ -n "$id" ] && mcp ui_click "{\"id\":\"$id\"}" >/dev/null; [ -n "$id" ]; }
shot() { xcrun simctl io booted screenshot "$OUT/$1.png" >/dev/null 2>&1; }
frames() { mcp frame_stats '{}' | grep -oE 'frames[^0-9]{1,6}[0-9]+' | grep -oE '[0-9]+$' | head -1; }

app=$(find dist -name '*.app' -maxdepth 3 | head -1)
log "app bundle: $app"
[ -n "$app" ] || { fail "no .app in dist"; exit 1; }
/usr/libexec/PlistBuddy -c Print "$app/Info.plist" > "$OUT/Info.plist.txt" 2>&1
grep -E "CFBundleIdentifier|NSLocalNetworkUsageDescription|MinimumOSVersion" -A0 "$OUT/Info.plist.txt" | tee -a "$OUT/steps.txt"
ls "$app" | head -40 > "$OUT/bundle.txt"

log "boot a simulator"
udid=$(xcrun simctl list devices available -j | python3 -c '
import json, sys
d = json.load(sys.stdin)["devices"]
for rt in sorted(d, reverse=True):
    if "iOS" not in rt:
        continue
    for dev in d[rt]:
        if dev["name"].startswith("iPhone"):
            print(dev["udid"]); sys.exit(0)')
echo "simulator: $udid" | tee -a "$OUT/steps.txt"
xcrun simctl boot "$udid" 2>&1 | tee -a "$OUT/steps.txt"
xcrun simctl bootstatus "$udid" -b > /dev/null 2>&1

log "install and launch"
xcrun simctl install booted "$app" 2>&1 | tee -a "$OUT/steps.txt"
xcrun simctl launch --console-pty booted "$BUNDLE" > "$OUT/console.txt" 2>&1 &
ready=0
for i in $(seq 1 60); do
  if mcp ui_snapshot '{"query":"Overview"}' | grep -q '"result"'; then ready=1; echo "app ready after ${i}x2s" | tee -a "$OUT/steps.txt"; break; fi
  sleep 2
done
[ "$ready" = 1 ] || { fail "the automation server never answered"; tail -50 "$OUT/console.txt"; exit 1; }
sleep 4
shot 01-overview
labels > "$OUT/overview.txt"

log "navigation: no Connections, Map or Security on iOS"
for page in Connections Map Security; do
  if grep -qx "button $page" "$OUT/overview.txt"; then fail "$page is in the navigation"; fi
done
grep -E "^button (Overview|Apps|Devices|Tools|History|Settings)$" "$OUT/overview.txt" | tee -a "$OUT/steps.txt"

log "traffic: the phone's interfaces show up"
curl -s -o /dev/null -m 30 https://speed.cloudflare.com/__down?bytes=20000000 || true
sleep 8
click Apps button
sleep 3
shot 02-apps
labels > "$OUT/apps.txt"
grep -E "Wi-Fi|Cellular|Ethernet" "$OUT/apps.txt" | head -5 | tee -a "$OUT/steps.txt"
grep -qE "^text (Wi-Fi|Cellular|Ethernet)$" "$OUT/apps.txt" || fail "no interface traffic in Apps"

log "tools: no traceroute, Wi-Fi analyzer or discovery"
click Tools button
sleep 2
shot 03-tools
labels > "$OUT/tools.txt"
for t in "Traceroute" "Wi-Fi analyzer" "Service discovery"; do
  if grep -q "$t" "$OUT/tools.txt"; then fail "$t is offered"; fi
done
grep -q "Speed test" "$OUT/tools.txt" || fail "the tools catalog is missing"

log "ping"
click Ping button && sleep 2   # opens the tool
click Ping button && sleep 8   # runs it
shot 05-ping
labels | grep -iE "ms|loss|method|reply|error|fail" | head -10 | tee -a "$OUT/steps.txt"

log "devices"
click Devices button
sleep 20
shot 06-devices
labels > "$OUT/devices.txt"; head -40 "$OUT/devices.txt" | tee -a "$OUT/steps.txt"
# The network card: this device, router (from the routing table), public IP.
grep -A1 -E "^text (This device|Router|Public IP)$" "$OUT/devices.txt" | tee -a "$OUT/steps.txt"

log "drawing"
mcp frame_stats '{"reset":true}' >/dev/null
click Overview button; sleep 4
n=$(frames); echo "frames: ${n:-?}" | tee -a "$OUT/steps.txt"
[ "${n:-0}" -gt 0 ] || fail "no frames drawn"

echo "--- console (last 40 lines)"; tail -40 "$OUT/console.txt"
exit $failed
