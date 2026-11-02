package dnslog

import "golang.org/x/net/bpf"

// snapLen is the most of a packet the filter passes up; it covers any UDP
// DNS response.
const snapLen = 65535

// packetOutgoing is Linux's PACKET_OUTGOING packet type.
const packetOutgoing = 4

// filterOptions describes where the IP header starts in captured packets.
type filterOptions struct {
	// offset is the length of the link-layer header before the IP header.
	offset uint32
	// ethernet means the link layer is Ethernet, whose EtherType tells IPv4
	// from IPv6; otherwise the IP version nibble does.
	ethernet bool
	// skipOutgoing drops packets this host sends (Linux only), so answers
	// on the loopback interface are not seen twice.
	skipOutgoing bool
}

// dnsFilter returns a classic BPF program that accepts UDP packets from
// port 53 over IPv4 or IPv6 and drops everything else in the kernel.
func dnsFilter(o filterOptions) ([]bpf.RawInstruction, error) {
	var a assembler
	L := o.offset
	if o.skipOutgoing {
		a.add(bpf.LoadExtension{Num: bpf.ExtType})
		a.jump(bpf.JumpEqual, packetOutgoing, "drop", "")
	}
	if o.ethernet {
		a.add(bpf.LoadAbsolute{Off: L - 2, Size: 2})
		a.jump(bpf.JumpEqual, 0x0800, "v4", "")
		a.jump(bpf.JumpEqual, 0x86dd, "v6", "drop")
	} else {
		a.add(bpf.LoadAbsolute{Off: L, Size: 1})
		a.add(bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0xf0})
		a.jump(bpf.JumpEqual, 0x40, "v4", "")
		a.jump(bpf.JumpEqual, 0x60, "v6", "drop")
	}
	a.label("v4")
	a.add(bpf.LoadAbsolute{Off: L + 9, Size: 1})
	a.jump(bpf.JumpEqual, protoUDP, "", "drop")
	a.add(bpf.LoadAbsolute{Off: L + 6, Size: 2})
	a.jump(bpf.JumpBitsSet, 0x1fff, "drop", "")
	a.add(bpf.LoadMemShift{Off: L})
	a.add(bpf.LoadIndirect{Off: L, Size: 2})
	a.jump(bpf.JumpEqual, dnsPort, "accept", "drop")
	a.label("v6")
	a.add(bpf.LoadAbsolute{Off: L + 6, Size: 1})
	a.jump(bpf.JumpEqual, protoUDP, "", "drop")
	a.add(bpf.LoadAbsolute{Off: L + 40, Size: 2})
	a.jump(bpf.JumpEqual, dnsPort, "accept", "drop")
	a.label("accept")
	a.add(bpf.RetConstant{Val: snapLen})
	a.label("drop")
	a.add(bpf.RetConstant{Val: 0})
	prog, err := a.build()
	if err != nil {
		return nil, err
	}
	return bpf.Assemble(prog)
}

// assembler builds a BPF program whose conditional jumps name labels.
type assembler struct {
	prog   []bpf.Instruction
	jumps  []pendingJump
	labels map[string]int
}

// pendingJump is a conditional jump whose targets are resolved in build.
type pendingJump struct {
	at              int
	ifTrue, ifFalse string
}

func (a *assembler) add(i bpf.Instruction) { a.prog = append(a.prog, i) }

// jump adds a conditional jump; an empty label means the next instruction.
func (a *assembler) jump(cond bpf.JumpTest, val uint32, ifTrue, ifFalse string) {
	a.jumps = append(a.jumps, pendingJump{len(a.prog), ifTrue, ifFalse})
	a.add(bpf.JumpIf{Cond: cond, Val: val})
}

func (a *assembler) label(name string) {
	if a.labels == nil {
		a.labels = map[string]int{}
	}
	a.labels[name] = len(a.prog)
}

// build resolves the jumps' labels into forward skips.
func (a *assembler) build() ([]bpf.Instruction, error) {
	for _, j := range a.jumps {
		ji := a.prog[j.at].(bpf.JumpIf)
		var err error
		if ji.SkipTrue, err = a.skip(j.at, j.ifTrue); err != nil {
			return nil, err
		}
		if ji.SkipFalse, err = a.skip(j.at, j.ifFalse); err != nil {
			return nil, err
		}
		a.prog[j.at] = ji
	}
	return a.prog, nil
}

func (a *assembler) skip(from int, label string) (uint8, error) {
	if label == "" {
		return 0, nil
	}
	to, ok := a.labels[label]
	if !ok {
		return 0, errUnknownLabel(label)
	}
	n := to - from - 1
	if n < 0 || n > 255 {
		return 0, errUnknownLabel(label)
	}
	return uint8(n), nil
}

// errUnknownLabel reports a jump to a missing or unreachable label.
type errUnknownLabel string

func (e errUnknownLabel) Error() string { return "dnslog: bad BPF jump to " + string(e) }
