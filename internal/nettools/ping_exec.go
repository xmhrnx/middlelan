package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// execPinger runs the operating system's ping binary once per request.
type execPinger struct {
	ip   net.IP
	path string
}

func newExecPinger(ip net.IP) (*execPinger, error) {
	name, _ := systemPingCommand(runtime.GOOS, ip, 0, time.Second)
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("system ping: %w", err)
	}
	return &execPinger{ip: ip, path: path}, nil
}

func (p *execPinger) method() string { return "system" }

func (p *execPinger) close() error { return nil }

func (p *execPinger) ping(ctx context.Context, seq int, timeout time.Duration) (PingReply, error) {
	reply := PingReply{Seq: seq, Addr: p.ip.String()}
	res, err := runSystemPing(ctx, p.ip, 0, timeout)
	if err != nil {
		return reply, err
	}
	if res.Reply {
		reply.OK, reply.RTT, reply.TTL = true, res.RTT, res.TTL
		if res.From != "" {
			reply.Addr = res.From
		}
	} else if res.TTLExceeded {
		reply.Err = "TTL exceeded at " + res.From
	}
	return reply, nil
}

// systemPingCommand builds the command line for a single system ping to ip,
// optionally limited to ttl hops (ttl <= 0 means no limit).
func systemPingCommand(goos string, ip net.IP, ttl int, timeout time.Duration) (string, []string) {
	v6 := ip.To4() == nil
	secs := int((timeout + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	target := ip.String()
	switch goos {
	case "windows":
		args := []string{"-n", "1", "-w", strconv.Itoa(int(timeout / time.Millisecond))}
		if ttl > 0 {
			args = append(args, "-i", strconv.Itoa(ttl))
		}
		if v6 {
			args = append(args, "-6")
		} else {
			args = append(args, "-4")
		}
		return "ping", append(args, target)
	case "darwin", "ios", "freebsd", "openbsd", "netbsd":
		if v6 {
			args := []string{"-n", "-c", "1"}
			if ttl > 0 {
				args = append(args, "-h", strconv.Itoa(ttl))
			}
			return "ping6", append(args, target)
		}
		args := []string{"-n", "-c", "1", "-t", strconv.Itoa(secs)}
		if ttl > 0 {
			args = append(args, "-m", strconv.Itoa(ttl))
		}
		return "ping", append(args, target)
	default: // linux, android
		name := "ping"
		if v6 && goos == "android" {
			name = "ping6"
		}
		args := []string{"-n", "-c", "1", "-W", strconv.Itoa(secs)}
		if ttl > 0 {
			args = append(args, "-t", strconv.Itoa(ttl))
		}
		return name, append(args, target)
	}
}

// runSystemPing executes one system ping and parses its output. The error is
// non-nil only when the binary could not be run at all.
func runSystemPing(ctx context.Context, ip net.IP, ttl int, timeout time.Duration) (pingOutput, error) {
	name, args := systemPingCommand(runtime.GOOS, ip, ttl, timeout)
	cctx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	start := time.Now()
	out, err := runCmd(cctx, name, args...)
	elapsed := time.Since(start)
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			if ctx.Err() != nil {
				return pingOutput{}, ctx.Err()
			}
			if cctx.Err() != nil {
				return pingOutput{}, nil // our own timeout: no answer
			}
			return pingOutput{}, fmt.Errorf("run %s: %w", name, err)
		}
		// Non-zero exit just means no (normal) reply; parse anyway.
	}
	res := parsePingOutput(string(out))
	if (res.Reply || res.TTLExceeded) && res.RTT == 0 {
		res.RTT = elapsed // approximate when the tool does not print it
	}
	return res, nil
}

// pingOutput is the parsed result of one system ping invocation.
type pingOutput struct {
	// From is the address that answered.
	From string
	// RTT and TTL are taken from the reply line when present.
	RTT time.Duration
	TTL int
	// Reply is true for a normal echo reply from the target.
	Reply bool
	// TTLExceeded is true when an intermediate router reported that the
	// TTL expired.
	TTLExceeded bool
}

var (
	reIPv4     = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	reIPv6     = regexp.MustCompile(`[0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){2,7}`)
	reTTLField = regexp.MustCompile(`(?i)\b(?:ttl|hlim)[=:]\s*(\d+)`)
	reTime     = regexp.MustCompile(`(?i)[=<]\s*([0-9]+(?:[.,][0-9]+)?)\s*ms`)
	reExceeded = regexp.MustCompile(`(?i)time to live exceeded|time exceeded|ttl expired|ttl abgelaufen|ttl expir|ttl scadut|ttl caduc|durée de vie|tempo de vida|ttl.*(?:exceed|expir|transit)`)
)

// firstIP returns the first IPv4 or IPv6 address found in s.
func firstIP(s string) string {
	v4 := reIPv4.FindStringIndex(s)
	for _, m := range reIPv6.FindAllStringIndex(s, -1) {
		cand := strings.Trim(s[m[0]:m[1]], ":")
		if strings.Count(cand, ":") < 2 && !strings.Contains(cand, "::") {
			continue
		}
		if ip := net.ParseIP(cand); ip != nil && ip.To4() == nil {
			if v4 == nil || m[0] < v4[0] {
				return ip.String()
			}
			break
		}
	}
	if v4 != nil {
		if ip := net.ParseIP(s[v4[0]:v4[1]]); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// parsePingOutput interprets the output of a single-packet system ping on
// Linux, Android, macOS or Windows (English and common localisations).
func parsePingOutput(out string) pingOutput {
	var res pingOutput
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if reExceeded.MatchString(line) {
			res.TTLExceeded = true
			res.From = firstIP(line)
			if m := reTime.FindStringSubmatch(line); m != nil {
				res.RTT = parseMs(m[1])
			}
			return res
		}
		ttlM := reTTLField.FindStringSubmatch(line)
		timeM := reTime.FindStringSubmatch(line)
		if ttlM != nil && timeM != nil {
			res.Reply = true
			res.From = firstIP(line)
			res.TTL, _ = strconv.Atoi(ttlM[1]) // regex guarantees digits
			res.RTT = parseMs(timeM[1])
			return res
		}
	}
	return res
}

// parseMs parses a millisecond value that may use a decimal comma.
func parseMs(s string) time.Duration {
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Millisecond))
}
