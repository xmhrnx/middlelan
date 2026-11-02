package monitor

import "syscall"

// linuxSource reads sockets through netlink sock_diag (with per-socket byte
// counters), falling back to /proc/net when sock_diag is refused.
type linuxSource struct {
	owners  *owners
	useDiag bool
	limits  string
}

func newLinuxSource() *linuxSource {
	return &linuxSource{owners: newOwners(), useDiag: true}
}

func (s *linuxSource) Sample() (Sample, error) {
	socks, perApp, err := s.sockets()
	if err != nil {
		return Sample{}, err
	}
	iface, err := readIfaceCounters()
	if err != nil {
		return Sample{}, err
	}
	return Sample{Conns: s.owners.resolve(socks), Iface: iface, PerApp: perApp}, nil
}

func (s *linuxSource) sockets() ([]diagSocket, bool, error) {
	if s.useDiag {
		var all []diagSocket
		var err error
		for _, q := range []struct{ fam, proto uint8 }{
			{syscall.AF_INET, syscall.IPPROTO_TCP}, {syscall.AF_INET6, syscall.IPPROTO_TCP},
			{syscall.AF_INET, syscall.IPPROTO_UDP}, {syscall.AF_INET6, syscall.IPPROTO_UDP},
		} {
			var part []diagSocket
			if part, err = diagDump(q.fam, q.proto); err != nil {
				break
			}
			all = append(all, part...)
		}
		if err == nil {
			return all, true, nil
		}
		s.useDiag = false
		s.limits = "Per-app bandwidth is unavailable: the system refused socket statistics (" + err.Error() + ")."
	}
	var all []diagSocket
	var firstErr error
	for _, name := range []string{"tcp", "tcp6", "udp", "udp6"} {
		part, err := readProcNet(name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, part...)
	}
	if len(all) == 0 && firstErr != nil {
		return nil, false, firstErr
	}
	return all, false, nil
}

func (s *linuxSource) Limits() string {
	if s.limits != "" {
		return s.limits
	}
	if !isRoot() {
		return "Connections of other users' and system processes show without a program name. Run as administrator to see them all."
	}
	return ""
}

func isRoot() bool { return syscall.Geteuid() == 0 }
