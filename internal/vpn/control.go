package vpn

// Default is the engine the Android VpnService drives through
// nativeStart and nativeStop. Set its hooks with SetHooks.
var Default = NewEngine(Hooks{})

// SetHooks sets Default's hooks. On Android an unset Owner hook is filled
// with the system's connection owner lookup (VpnBridge.ownerUid and
// appLabel, cached per uid).
func SetHooks(h Hooks) { Default.SetHooks(platformHooks(h)) }
