//go:build windows

package firewall

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// firewallServiceRunning reports whether the Windows Firewall service
// (MpsSvc) is running. ok is false when the state couldn't be read.
func firewallServiceRunning() (running, ok bool) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, false
	}
	defer windows.CloseServiceHandle(m)
	name, err := windows.UTF16PtrFromString("MpsSvc")
	if err != nil {
		return false, false
	}
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false, false
	}
	defer windows.CloseServiceHandle(s)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(s, &st); err != nil {
		return false, false
	}
	return st.CurrentState == windows.SERVICE_RUNNING, true
}

// allProfilesOff reports whether the domain, private and public profiles
// are all turned off, reading group policy first and local settings second.
// It only reads the registry; it never changes a profile.
func allProfilesOff() bool {
	const (
		policy = `SOFTWARE\Policies\Microsoft\WindowsFirewall\`
		local  = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\`
	)
	for _, p := range []struct{ policy, local string }{
		{"DomainProfile", "DomainProfile"}, {"PrivateProfile", "StandardProfile"}, {"PublicProfile", "PublicProfile"},
	} {
		v, ok := readDWORD(policy+p.policy, "EnableFirewall")
		if !ok {
			v, ok = readDWORD(local+p.local, "EnableFirewall")
		}
		if !ok || v != 0 {
			return false
		}
	}
	return true
}

// readDWORD reads a DWORD value under HKEY_LOCAL_MACHINE.
func readDWORD(path, name string) (uint64, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return v, true
}
