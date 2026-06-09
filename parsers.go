package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RouteServerParser struct {
	birdRe     *regexp.Regexp
	routerRe   *regexp.Regexp
	serverRe   *regexp.Regexp
	rebootRe   *regexp.Regexp
	reconfigRe *regexp.Regexp
}

func NewRouteServerParser() *RouteServerParser {
	return &RouteServerParser{
		birdRe:     regexp.MustCompile(`^BIRD\s(?:(\w+)/)?([0-9a-zA-Z/\-.]+).*`),
		routerRe:   regexp.MustCompile(`^Router\sID\sis\s([0-9.]+).*`),
		serverRe:   regexp.MustCompile(`^Current\sserver\stime\sis\s([0-9\-]+\s[0-9:]+).*`),
		rebootRe:   regexp.MustCompile(`^Last\sreboot\son\s([0-9\-]+\s[0-9:]+).*`),
		reconfigRe: regexp.MustCompile(`^Last\sreconfiguration\son\s([0-9\-]+\s[0-9:]+).*`),
	}
}

func (p *RouteServerParser) Update(server *RouteServer, data string) {
	for _, line := range splitLines(data) {
		switch {
		case p.birdRe.MatchString(line):
			m := p.birdRe.FindStringSubmatch(line)
			server.Version = m[2]
		case p.routerRe.MatchString(line):
			m := p.routerRe.FindStringSubmatch(line)
			server.RouterID = m[1]
		case p.serverRe.MatchString(line):
			m := p.serverRe.FindStringSubmatch(line)
			server.ServerTime = parseBirdTime(m[1])
		case p.rebootRe.MatchString(line):
			m := p.rebootRe.FindStringSubmatch(line)
			server.LastReboot = parseBirdTime(m[1])
		case p.reconfigRe.MatchString(line):
			m := p.reconfigRe.FindStringSubmatch(line)
			server.LastReconfiguration = parseBirdTime(m[1])
		case strings.TrimSpace(line) != "":
			server.Message = line
		}
	}
}

type SymbolParser struct {
	symbolRe *regexp.Regexp
}

func NewSymbolParser() *SymbolParser {
	return &SymbolParser{symbolRe: regexp.MustCompile(`^([^\s]+)\s+(.+)$`)}
}

func (p *SymbolParser) Symbols(data string) map[string][]Symbol {
	results := map[string][]Symbol{
		"protocol": {},
		"table":    {},
	}
	for _, line := range splitLines(data) {
		m := p.symbolRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		switch m[2] {
		case "protocol":
			results["protocol"] = append(results["protocol"], Symbol{ID: m[1], Type: "protocol"})
		case "routing table":
			results["table"] = append(results["table"], Symbol{ID: m[1], Type: "table"})
		}
	}
	return results
}

type BgpProtocolParser struct {
	headerRe       *regexp.Regexp
	descriptionRe  *regexp.Regexp
	tableRe        *regexp.Regexp
	importLimitRe  *regexp.Regexp
	routesRe       *regexp.Regexp
	bgpStateRe     *regexp.Regexp
	neighborAddrRe *regexp.Regexp
	neighborASRe   *regexp.Regexp
	routeLimitRe   *regexp.Regexp
}

func NewBgpProtocolParser() *BgpProtocolParser {
	return &BgpProtocolParser{
		headerRe:       regexp.MustCompile(`^(\w+)\s+BGP\s+([-\w]+)\s+(\w+)\s+([0-9\-]+\s[0-9:]+)(?:\.\d+)?\s+(\w+).*`),
		descriptionRe:  regexp.MustCompile(`^\s+Description:\s+(.*)$`),
		tableRe:        regexp.MustCompile(`^\s+Table:\s+(.*)$`),
		importLimitRe:  regexp.MustCompile(`^\s+Import limit:\s+([0-9]+).*`),
		routesRe:       regexp.MustCompile(`^\s+Routes:\s+([0-9]+)\s+imported,\s+(?:([0-9]+)\s+filtered,\s+)*([0-9]+)\s+exported,\s+([0-9]+)\s+preferred.*`),
		bgpStateRe:     regexp.MustCompile(`^\s+BGP state:\s+(\w+).*`),
		neighborAddrRe: regexp.MustCompile(`^\s+Neighbor address:\s+([^\s]+).*`),
		neighborASRe:   regexp.MustCompile(`^\s+Neighbor AS:\s+([0-9]+).*`),
		routeLimitRe:   regexp.MustCompile(`^\s+Route limit:\s+([0-9]+)/([0-9]+).*`),
	}
}

func (p *BgpProtocolParser) Protocols(data string) []*BgpProtocol {
	blobs := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n\n")
	protocols := []*BgpProtocol{}
	for _, blob := range blobs {
		if !regexp.MustCompile(`^\w+\s+BGP\s+.*`).MatchString(blob) {
			continue
		}
		protocol := p.parseBlob(blob)
		if protocol != nil {
			protocols = append(protocols, protocol)
		}
	}
	return protocols
}

func (p *BgpProtocolParser) parseBlob(blob string) *BgpProtocol {
	protocol := &BgpProtocol{ID: newID(), Blob: blob}
	matched := false
	for _, line := range splitLines(blob) {
		switch {
		case p.headerRe.MatchString(line):
			m := p.headerRe.FindStringSubmatch(line)
			protocol.Name = m[1]
			protocol.Table = m[2]
			protocol.State = m[3]
			protocol.StateChanged = parseBirdTime(m[4])
			matched = true
		case p.descriptionRe.MatchString(line):
			m := p.descriptionRe.FindStringSubmatch(line)
			protocol.Description = strings.TrimSpace(m[1])
			matched = true
		case p.tableRe.MatchString(line):
			m := p.tableRe.FindStringSubmatch(line)
			protocol.Table = strings.TrimSpace(m[1])
			matched = true
		case p.importLimitRe.MatchString(line):
			m := p.importLimitRe.FindStringSubmatch(line)
			protocol.ImportLimit = intPtr(atoi(m[1]))
			matched = true
		case p.routesRe.MatchString(line):
			m := p.routesRe.FindStringSubmatch(line)
			protocol.ImportedRoutes = intPtr(atoi(m[1]))
			protocol.ExportedRoutes = intPtr(atoi(m[3]))
			matched = true
		case p.bgpStateRe.MatchString(line):
			m := p.bgpStateRe.FindStringSubmatch(line)
			protocol.BgpState = strings.TrimSpace(m[1])
			matched = true
		case p.neighborAddrRe.MatchString(line):
			m := p.neighborAddrRe.FindStringSubmatch(line)
			protocol.NeighborAddress = m[1]
			matched = true
		case p.neighborASRe.MatchString(line):
			m := p.neighborASRe.FindStringSubmatch(line)
			protocol.ASN = atoi(m[1])
			matched = true
		case p.routeLimitRe.MatchString(line):
			m := p.routeLimitRe.FindStringSubmatch(line)
			protocol.RouteLimit = intPtr(atoi(m[1]))
			matched = true
		}
	}
	if !matched {
		return nil
	}
	return protocol
}

type BfdSessionParser struct {
	sessionRe *regexp.Regexp
}

func NewBfdSessionParser() *BfdSessionParser {
	return &BfdSessionParser{
		sessionRe: regexp.MustCompile(`^([0-9a-fA-F.:/]+)\s+([0-9a-zA-Z_.-]+)\s+(\w+)\s+([0-9\-]+\s[0-9:]+)(?:\.\d+)?\s+([0-9.]+)\s+([0-9.]+).*`),
	}
}

func (p *BfdSessionParser) Sessions(data string) []BfdSession {
	sessions := []BfdSession{}
	for _, line := range splitLines(data) {
		m := p.sessionRe.FindStringSubmatch(line)
		if len(m) != 7 {
			continue
		}
		sessions = append(sessions, BfdSession{
			IPAddress: m[1],
			Interface: m[2],
			State:     m[3],
			Since:     parseBirdTime(m[4]),
			Interval:  atof(m[5]),
			Timeout:   atof(m[6]),
		})
	}
	return sessions
}

type flagPattern struct {
	regexp *regexp.Regexp
	flag   Flag
}

type RouteTableParser struct {
	cfg                    *Config
	flags                  map[string]Flag
	communityPatterns      []flagPattern
	largeCommunityPatterns []flagPattern
	extCommunityPatterns   []flagPattern
	firstRouteRe           *regexp.Regexp
	nextRouteRe            *regexp.Regexp
	asPathRe               *regexp.Regexp
	nextHopRe              *regexp.Regexp
	communityRe            *regexp.Regexp
	largeCommunityRe       *regexp.Regexp
	extCommunityRe         *regexp.Regexp
	tableRe                *regexp.Regexp
}

func NewRouteTableParser(cfg *Config) *RouteTableParser {
	return &RouteTableParser{
		cfg:                    cfg,
		flags:                  cfg.Flags,
		communityPatterns:      compilePatterns(cfg.FlagPatterns.Communities, cfg.Flags),
		largeCommunityPatterns: compilePatterns(cfg.FlagPatterns.LargeCommunities, cfg.Flags),
		extCommunityPatterns:   compilePatterns(cfg.FlagPatterns.ExtendedCommunities, cfg.Flags),
		firstRouteRe:           regexp.MustCompile(`^([0-9a-fA-F.:/]+)\s+.+\s+\[([^\s\]]+)\s+[^\]]+\]\s+(\*)?\s*\(([0-9]+)`),
		nextRouteRe:            regexp.MustCompile(`^\s+.+\s+\[([^\s\]]+)\s+[^\]]+\]\s+(\*)?\s*\(([0-9]+)`),
		asPathRe:               regexp.MustCompile(`^\s+BGP\.as_path:\s+(.*)$`),
		nextHopRe:              regexp.MustCompile(`^\s+BGP\.next_hop:\s+([0-9a-fA-F.:/]+).*`),
		communityRe:            regexp.MustCompile(`\(([0-9]+),\s*([0-9]+)\)`),
		largeCommunityRe:       regexp.MustCompile(`\(([0-9]+),\s*([0-9]+),\s*([0-9]+)\)`),
		extCommunityRe:         regexp.MustCompile(`\((\w+),\s*(\w+),\s*(\w+)\)`),
		tableRe:                regexp.MustCompile(`^Table\s+(\w+):$`),
	}
}

func (p *RouteTableParser) RouteTables(data string) []*RouteTable {
	name := ""
	var blob strings.Builder
	tables := []*RouteTable{}
	for _, line := range splitLines(data) {
		if m := p.tableRe.FindStringSubmatch(line); len(m) == 2 {
			if strings.TrimSpace(blob.String()) != "" {
				tables = append(tables, &RouteTable{ID: newID(), Name: name, Blob: blob.String(), Routes: map[string]*Route{}})
				blob.Reset()
			}
			name = m[1]
			continue
		}
		if strings.TrimSpace(line) != "" {
			blob.WriteString(line)
			blob.WriteByte('\n')
		}
	}
	if strings.TrimSpace(blob.String()) != "" {
		tables = append(tables, &RouteTable{ID: newID(), Name: name, Blob: blob.String(), Routes: map[string]*Route{}})
	}
	return tables
}

func (p *RouteTableParser) Routes(data string) []*Route {
	routes := []*Route{}
	route := newRoute()
	if flag, ok := p.flags["valid"]; ok {
		route.AddFlag(flag)
	}
	prefix := ""
	var blob strings.Builder

	save := func() {
		if strings.TrimSpace(blob.String()) == "" {
			return
		}
		route.Blob = blob.String()
		routes = append(routes, route)
		route = newRoute()
		if flag, ok := p.flags["valid"]; ok {
			route.AddFlag(flag)
		}
		blob.Reset()
	}

	for _, line := range splitLines(data) {
		if m := p.firstRouteRe.FindStringSubmatch(line); len(m) == 5 {
			save()
			prefix = m[1]
			route.Network = prefix
			route.FromProtocol = m[2]
			route.Primary = m[3] == "*"
			route.Metric = atoi(m[4])
			if route.Primary {
				if flag, ok := p.flags["primary"]; ok {
					route.AddFlag(flag)
				}
			} else if flag, ok := p.flags["secondary"]; ok {
				route.AddFlag(flag)
			}
			blob.WriteString(line)
			blob.WriteByte('\n')
			continue
		}

		if m := p.nextRouteRe.FindStringSubmatch(line); len(m) == 4 {
			save()
			route.Network = prefix
			route.FromProtocol = m[1]
			route.Primary = m[2] == "*"
			route.Metric = atoi(m[3])
			if route.Primary {
				if flag, ok := p.flags["primary"]; ok {
					route.AddFlag(flag)
				}
			} else if flag, ok := p.flags["secondary"]; ok {
				route.AddFlag(flag)
			}
			blob.WriteString(prefix)
			blob.WriteString(line[min(len(line), len(prefix)):])
			blob.WriteByte('\n')
			continue
		}

		if m := p.asPathRe.FindStringSubmatch(line); len(m) == 2 {
			route.ASPath = strings.Fields(strings.TrimSpace(m[1]))
		}
		if m := p.nextHopRe.FindStringSubmatch(line); len(m) == 2 {
			route.NextHop = m[1]
		}
		if strings.Contains(line, "BGP.community:") || p.communityRe.MatchString(line) {
			p.parseCommunities(route, line)
		}
		if strings.Contains(line, "BGP.large_community:") || p.largeCommunityRe.MatchString(line) {
			p.parseLargeCommunities(route, line)
		}
		if strings.Contains(line, "BGP.ext_community:") || p.extCommunityRe.MatchString(line) {
			p.parseExtendedCommunities(route, line)
		}
		if strings.TrimSpace(line) != "" {
			blob.WriteString(line)
			blob.WriteByte('\n')
		}
	}
	save()
	return routes
}

func (p *RouteTableParser) CountByTable(data string) map[string]int {
	counts := map[string]int{}
	for _, table := range p.RouteTables(data) {
		if !isRouteTable(table.Name) {
			continue
		}
		counts[table.Name] = len(p.Routes(table.Blob))
	}
	return counts
}

func (p *RouteTableParser) parseCommunities(route *Route, line string) {
	for _, m := range p.communityRe.FindAllStringSubmatch(line, -1) {
		key := m[1] + ":" + m[2]
		route.Communities = append(route.Communities, p.communityValue(key, "community"))
		for _, item := range p.communityPatterns {
			if item.regexp.MatchString(key) {
				route.AddFlag(item.flag)
			}
		}
	}
}

func (p *RouteTableParser) parseLargeCommunities(route *Route, line string) {
	for _, m := range p.largeCommunityRe.FindAllStringSubmatch(line, -1) {
		key := m[1] + ":" + m[2] + ":" + m[3]
		route.LargeCommunities = append(route.LargeCommunities, p.communityValue(key, "large"))
		for _, item := range p.largeCommunityPatterns {
			if item.regexp.MatchString(key) {
				route.AddFlag(item.flag)
			}
		}
	}
}

func (p *RouteTableParser) parseExtendedCommunities(route *Route, line string) {
	for _, m := range p.extCommunityRe.FindAllStringSubmatch(line, -1) {
		key := m[1] + ":" + m[2] + ":" + m[3]
		route.ExtendedCommunities = append(route.ExtendedCommunities, p.communityValue(key, "extended"))
		for _, item := range p.extCommunityPatterns {
			if item.regexp.MatchString(key) {
				route.AddFlag(item.flag)
			}
		}
	}
}

func (p *RouteTableParser) communityValue(key, kind string) CommunityValue {
	raw := rawCommunity(key, kind)
	known := p.knownCommunity(key, kind)
	return CommunityValue{ID: key, Raw: raw, Name: known.Name, Label: known.Label}
}

func (p *RouteTableParser) knownCommunity(key, kind string) CommunityInfo {
	maps := p.cfg.Known.Communities
	if kind == "large" {
		maps = p.cfg.Known.LargeCommunities
	}
	if kind == "extended" {
		maps = p.cfg.Known.ExtendedCommunities
	}
	if known, ok := maps[key]; ok {
		return known
	}
	parts := strings.Split(key, ":")
	if len(parts) == 2 {
		if known, ok := maps[parts[0]+":*"]; ok {
			return known
		}
	}
	if len(parts) == 3 {
		if known, ok := maps[parts[0]+":"+parts[1]+":*"]; ok {
			return known
		}
		if known, ok := maps[parts[0]+":*:*"]; ok {
			return known
		}
	}
	return CommunityInfo{}
}

func rawCommunity(key, kind string) string {
	parts := strings.Split(key, ":")
	if len(parts) == 2 {
		return "(" + parts[0] + "," + parts[1] + ")"
	}
	if len(parts) == 3 {
		return "(" + parts[0] + ", " + parts[1] + ", " + parts[2] + ")"
	}
	return key
}

func isRouteTable(name string) bool {
	return strings.HasPrefix(name, "T4_") || strings.HasPrefix(name, "T6_") || name == "master4" || name == "master6"
}

func intPtr(v int) *int {
	return &v
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func atof(value string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = time.Second
