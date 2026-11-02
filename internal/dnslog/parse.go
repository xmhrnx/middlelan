package dnslog

import (
	"net/netip"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// maxChain bounds how many CNAME hops are followed from a query name.
const maxChain = 16

// maxQuestions bounds how many questions of one message are considered.
const maxQuestions = 4

// record is an address record found in a message, by owner name.
type record struct {
	owner string
	addr  netip.Addr
}

// alias is a CNAME record: owner is an alias of target.
type alias struct{ owner, target string }

// ParseResponse extracts answers from a raw DNS response message: for each
// question, the A and AAAA addresses it resolved to, following CNAMEs in
// the same message, are reported under the query name (the name the app
// asked for). Address records not reachable from a question are reported
// under their own name. Queries, failed responses and malformed messages
// yield nothing; a truncated message yields the answers before the cut.
func ParseResponse(msg []byte, at time.Time) []Answer {
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil || !h.Response || h.RCode != dnsmessage.RCodeSuccess {
		return nil
	}
	var questions []string
	for {
		q, err := p.Question()
		if err != nil {
			if err != dnsmessage.ErrSectionDone {
				return nil
			}
			break
		}
		if len(questions) < maxQuestions && (q.Type == dnsmessage.TypeA || q.Type == dnsmessage.TypeAAAA) {
			questions = append(questions, canonicalName(q.Name.String()))
		}
	}
	records, aliases := parseAnswers(&p)
	if len(records) == 0 {
		return nil
	}
	answers := make([]Answer, 0, len(records))
	for _, r := range records {
		answers = append(answers, Answer{Name: queryName(r.owner, questions, aliases), IP: r.addr.Unmap().String(), At: at})
	}
	return answers
}

// parseAnswers reads the answer section, keeping address and CNAME records.
// It stops quietly at the first malformed record.
func parseAnswers(p *dnsmessage.Parser) ([]record, []alias) {
	var records []record
	var aliases []alias
	for {
		hdr, err := p.AnswerHeader()
		if err != nil {
			return records, aliases
		}
		if hdr.Class != dnsmessage.ClassINET {
			if p.SkipAnswer() != nil {
				return records, aliases
			}
			continue
		}
		switch hdr.Type {
		case dnsmessage.TypeA:
			r, err := p.AResource()
			if err != nil {
				return records, aliases
			}
			records = append(records, record{canonicalName(hdr.Name.String()), netip.AddrFrom4(r.A)})
		case dnsmessage.TypeAAAA:
			r, err := p.AAAAResource()
			if err != nil {
				return records, aliases
			}
			records = append(records, record{canonicalName(hdr.Name.String()), netip.AddrFrom16(r.AAAA)})
		case dnsmessage.TypeCNAME:
			r, err := p.CNAMEResource()
			if err != nil {
				return records, aliases
			}
			aliases = append(aliases, alias{canonicalName(hdr.Name.String()), canonicalName(r.CNAME.String())})
		default:
			if p.SkipAnswer() != nil {
				return records, aliases
			}
		}
	}
}

// queryName returns the question whose CNAME chain leads to owner, or owner
// itself when none does.
func queryName(owner string, questions []string, aliases []alias) string {
	for _, q := range questions {
		name := q
		for hop := 0; hop <= maxChain; hop++ {
			if name == owner {
				return q
			}
			next, ok := cnameOf(name, aliases)
			if !ok {
				break
			}
			name = next
		}
	}
	return owner
}

// cnameOf returns the CNAME target of name, if the message has one.
func cnameOf(name string, aliases []alias) (string, bool) {
	for _, a := range aliases {
		if a.owner == name {
			return a.target, true
		}
	}
	return "", false
}
