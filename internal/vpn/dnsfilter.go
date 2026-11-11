package vpn

import (
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// questionName returns the name a DNS query asks about, lowercase and
// without the final dot; ok is false for anything but a query with one
// question.
func questionName(q []byte) (string, bool) {
	var p dnsmessage.Parser
	h, err := p.Start(q)
	if err != nil || h.Response {
		return "", false
	}
	qs, err := p.AllQuestions()
	if err != nil || len(qs) != 1 {
		return "", false
	}
	return strings.TrimSuffix(strings.ToLower(qs[0].Name.String()), "."), true
}

// refusedReply answers a filtered query: the unspecified address for A
// and AAAA (what hosts-file blockers answer, so apps fail fast instead of
// retrying), NXDOMAIN for any other type. nil if q doesn't parse.
func refusedReply(q []byte) []byte {
	var p dnsmessage.Parser
	h, err := p.Start(q)
	if err != nil {
		return nil
	}
	qs, err := p.AllQuestions()
	if err != nil || len(qs) != 1 {
		return nil
	}
	question := qs[0]
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID: h.ID, Response: true, OpCode: h.OpCode,
			RecursionDesired: h.RecursionDesired, RecursionAvailable: true,
		},
		Questions: []dnsmessage.Question{question},
	}
	rh := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET, TTL: 300}
	switch question.Type {
	case dnsmessage.TypeA:
		msg.Answers = []dnsmessage.Resource{{Header: rh, Body: &dnsmessage.AResource{}}}
	case dnsmessage.TypeAAAA:
		msg.Answers = []dnsmessage.Resource{{Header: rh, Body: &dnsmessage.AAAAResource{}}}
	default:
		msg.Header.RCode = dnsmessage.RCodeNameError
	}
	b, err := msg.Pack()
	if err != nil {
		return nil
	}
	return b
}
