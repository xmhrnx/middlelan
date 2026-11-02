package threat

// source is one downloadable blocklist.
type source struct {
	Name     string
	URL      string
	File     string // cache file name under dir/threat
	Category Category
	// Domains allows host names as entries (hosts-file style lists).
	Domains bool
	// MinEntries rejects a download with fewer usable entries, which
	// would mean a truncated or changed file rather than a real list.
	MinEntries int
}

// defaultSources returns the built-in lists, most specific first: a
// connection matching several lists reports the first one.
//
// No phishing list is included: the free ones are either very large or
// list URLs on shared hosting (docs.google.com, github.io ...), which
// would flag ordinary traffic when reduced to host names.
func defaultSources() []*source {
	return []*source{
		{
			Name: "abuse.ch Feodo Tracker", URL: "https://feodotracker.abuse.ch/downloads/ipblocklist.txt",
			File: "feodo.txt", Category: CategoryBotnet,
			MinEntries: 0, // the list can legitimately be (nearly) empty
		},
		{
			Name: "abuse.ch URLhaus", URL: "https://urlhaus.abuse.ch/downloads/hostfile/",
			File: "urlhaus.txt", Category: CategoryMalware, Domains: true, MinEntries: 10,
		},
		{
			Name: "Spamhaus DROP", URL: "https://www.spamhaus.org/drop/drop.txt",
			File: "spamhaus_drop.txt", Category: CategorySpam, MinEntries: 100,
		},
		{
			Name: "Spamhaus DROPv6", URL: "https://www.spamhaus.org/drop/drop_v6.txt",
			File: "spamhaus_drop_v6.txt", Category: CategorySpam, MinEntries: 5,
		},
		{
			Name: "FireHOL level1", URL: "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset",
			File: "firehol_level1.txt", Category: CategoryCompromised, MinEntries: 500,
		},
	}
}
