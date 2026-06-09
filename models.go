package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Flag struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Label  string `json:"label"`
	Type   string `json:"type"`
	Weight int    `json:"weight"`
}

type RouteServer struct {
	ID                  string
	Name                string
	Description         string
	Socket              string
	Version             string
	VersionMask         string
	RouterID            string
	ServerTime          *time.Time
	LastReboot          *time.Time
	LastReconfiguration *time.Time
	Message             string
	CreatedAt           time.Time
}

func (s RouteServer) DisplayVersion() string {
	if s.Version != "" {
		return s.Version
	}
	return s.VersionMask
}

func (s RouteServer) UptimeDays(now time.Time) int {
	if s.LastReboot == nil {
		return 0
	}
	return int(now.Sub(*s.LastReboot).Hours() / 24)
}

type Peer struct {
	Name        string
	ASN         any
	Table       string
	IP          string
	Description string
}

func defaultPeer() Peer {
	return Peer{
		Name:        "n/a",
		ASN:         "n/a",
		Table:       "n/a",
		IP:          "n/a",
		Description: "n/a",
	}
}

func (p Peer) Protocol() string {
	if len(p.Table) > 1 {
		return "R" + p.Table[1:]
	}
	return "n/a"
}

func (p Peer) DisplayName() string {
	if p.Table == "master4" || p.Table == "master6" {
		return ""
	}
	if p.Name == "" {
		return "n/a"
	}
	return p.Name
}

func (p Peer) DisplayDescription() string {
	if p.Table == "master4" || p.Table == "master6" {
		return p.Table
	}
	if p.Description == "" {
		return "n/a"
	}
	return p.Description
}

type Symbol struct {
	ID   string
	Type string
	Peer Peer
}

type BgpProtocol struct {
	ID              string
	Name            string
	State           string
	StateChanged    *time.Time
	Description     string
	BgpState        string
	NeighborAddress string
	ASN             int
	Table           string
	ImportLimit     *int
	RouteLimit      *int
	ImportedRoutes  *int
	ExportedRoutes  *int
	SelectedRoutes  int
	InvalidRoutes   int
	Blob            string
	CreatedAt       time.Time
}

func (p BgpProtocol) PeerName() string {
	parts := strings.Split(p.Description, " - ")
	if len(parts) > 0 {
		return parts[0]
	}
	return p.Description
}

func (p BgpProtocol) FormattedDescription() string {
	version := "IPv4"
	if strings.HasPrefix(p.Table, "T6_") {
		version = "IPv6"
	}
	number := strings.TrimPrefix(strings.TrimPrefix(p.Table, "T4_"), "T6_")
	name := p.PeerName()
	if name == "" {
		name = "n/a"
	}
	return fmt.Sprintf("%s %s (%s)", name, version, number)
}

func (p BgpProtocol) BgpStateShortcut() string {
	if len(p.BgpState) <= 3 {
		return p.BgpState
	}
	return p.BgpState[:3]
}

func (p BgpProtocol) Highlighted() bool {
	return p.State != "up"
}

func (p BgpProtocol) routeLimit() int {
	if p.RouteLimit != nil {
		return *p.RouteLimit
	}
	if p.ImportedRoutes != nil {
		return *p.ImportedRoutes
	}
	return 0
}

func (p BgpProtocol) importRatio() any {
	if p.Highlighted() || p.ImportLimit == nil || *p.ImportLimit == 0 {
		return nil
	}
	return float64(p.routeLimit()) / float64(*p.ImportLimit)
}

func (p BgpProtocol) invalidRatio() any {
	if p.Highlighted() || p.ImportedRoutes == nil || *p.ImportedRoutes == 0 {
		return nil
	}
	return float64(p.InvalidRoutes) / float64(*p.ImportedRoutes)
}

func (p BgpProtocol) ToJSON() map[string]any {
	return map[string]any{
		"id":              p.ID,
		"peer_name":       p.PeerName(),
		"table":           p.Table,
		"protocol":        p.Name,
		"ip_address":      p.NeighborAddress,
		"description":     p.Description,
		"asn":             p.ASN,
		"bgp_state":       map[string]any{"value": p.BgpState, "shortcut": p.BgpStateShortcut()},
		"import_limit":    intPtrValue(p.ImportLimit),
		"import_ratio":    p.importRatio(),
		"imported_routes": nullableEstablishedInt(p.BgpState, p.ImportedRoutes, p.Highlighted()),
		"exported_routes": nullableEstablishedInt(p.BgpState, p.ExportedRoutes, p.Highlighted()),
		"selected_routes": nullableInt(p.SelectedRoutes, p.Highlighted()),
		"invalid_routes":  nullableInt(p.InvalidRoutes, p.Highlighted()),
		"invalid_ratio":   p.invalidRatio(),
		"state":           p.State,
		"state_changed":   timeJSON(p.StateChanged),
		"highlighted":     p.Highlighted(),
	}
}

type BfdSession struct {
	IPAddress string
	Interface string
	State     string
	Since     *time.Time
	Interval  float64
	Timeout   float64
	Peer      Peer
}

func (s BfdSession) ToJSON() map[string]any {
	return map[string]any{
		"peer_name":   s.Peer.DisplayName(),
		"table":       s.Peer.Table,
		"ip_address":  s.IPAddress,
		"description": s.Peer.DisplayDescription(),
		"asn":         s.Peer.ASN,
		"interface":   s.Interface,
		"state":       s.State,
		"since":       timeJSON(s.Since),
		"interval":    s.Interval,
		"timeout":     s.Timeout,
	}
}

type RouteTable struct {
	ID        string
	Name      string
	Blob      string
	Peer      Peer
	Routes    map[string]*Route
	CreatedAt time.Time
}

type CommunityValue struct {
	ID    string `json:"id"`
	Raw   string `json:"raw"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

type Route struct {
	ID                  string
	TableName           string
	TableID             string
	PeerName            string
	PeerASN             any
	Description         string
	Network             string
	NeighborName        string
	NeighborASN         any
	NextHop             string
	FromProtocol        string
	Primary             bool
	Flags               map[string]Flag
	Metric              int
	Communities         []CommunityValue
	LargeCommunities    []CommunityValue
	ExtendedCommunities []CommunityValue
	ASPath              []string
	Blob                string
}

func newRoute() *Route {
	return &Route{
		ID:           newID(),
		NeighborName: "n/a",
		NeighborASN:  "n/a",
		Flags:        map[string]Flag{},
	}
}

func (r *Route) AddFlag(flag Flag) {
	if flag.ID == "" {
		return
	}
	if flag.ID == "invalid" {
		delete(r.Flags, "valid")
	}
	r.Flags[flag.ID] = flag
}

func (r Route) sortedFlags() []Flag {
	flags := make([]Flag, 0, len(r.Flags))
	for _, flag := range r.Flags {
		flags = append(flags, flag)
	}
	sort.Slice(flags, func(i, j int) bool {
		if flags[i].Weight == flags[j].Weight {
			return flags[i].ID < flags[j].ID
		}
		return flags[i].Weight < flags[j].Weight
	})
	return flags
}

func (r Route) highlighted() bool {
	_, ok := r.Flags["invalid"]
	return ok
}

func (r Route) background() string {
	if _, ok := r.Flags["rpki_invalid"]; ok {
		return "red"
	}
	if _, ok := r.Flags["invalid"]; ok {
		return "red"
	}
	return "success"
}

func (r Route) ToJSON() map[string]any {
	return map[string]any{
		"id":                   r.ID,
		"table_id":             r.TableID,
		"table_name":           r.TableName,
		"peer_name":            r.PeerName,
		"peer_asn":             r.PeerASN,
		"description":          r.Description,
		"network":              r.Network,
		"next_hop":             r.NextHop,
		"neighbor_name":        r.NeighborName,
		"neighbor_asn":         r.NeighborASN,
		"primary":              r.Primary,
		"flags":                r.sortedFlags(),
		"metric":               r.Metric,
		"communities":          communitiesJSON(r.Communities, communityFilters),
		"large_communities":    communitiesJSON(r.LargeCommunities, largeCommunityFilters),
		"extended_communities": communitiesJSON(r.ExtendedCommunities, extendedCommunityFilters),
		"as_path":              r.ASPath,
		"highlighted":          r.highlighted(),
		"background":           r.background(),
	}
}

func communitiesJSON(values []CommunityValue, filters func(CommunityValue) []string) map[string]any {
	sorted := append([]CommunityValue(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return naturalCommunityKey(sorted[i].ID) < naturalCommunityKey(sorted[j].ID)
	})
	filterValues := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		for _, key := range filters(value) {
			if !seen[key] {
				filterValues = append(filterValues, key)
				seen[key] = true
			}
		}
	}
	return map[string]any{
		"count":         len(values),
		"values":        sorted,
		"filter_values": filterValues,
	}
}

func naturalCommunityKey(id string) string {
	parts := strings.Split(id, ":")
	for i, part := range parts {
		if n, err := strconv.Atoi(part); err == nil {
			parts[i] = fmt.Sprintf("%020d", n)
		}
	}
	return strings.Join(parts, ":")
}

func communityFilters(value CommunityValue) []string {
	parts := strings.Split(value.ID, ":")
	if len(parts) != 2 {
		return nil
	}
	return []string{value.ID, parts[0] + ":*"}
}

func largeCommunityFilters(value CommunityValue) []string {
	parts := strings.Split(value.ID, ":")
	if len(parts) != 3 {
		return nil
	}
	return []string{value.ID, parts[0] + ":" + parts[1] + ":*", parts[0] + ":*:*"}
}

func extendedCommunityFilters(value CommunityValue) []string {
	parts := strings.Split(value.ID, ":")
	if len(parts) != 3 {
		return nil
	}
	return []string{value.ID, parts[0] + ":" + parts[1] + ":*", parts[0] + ":*:*"}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexed[0:8], hexed[8:12], hexed[12:16], hexed[16:20], hexed[20:32])
}

func parseBirdTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, "."); idx >= 0 {
		value = value[:idx]
	}
	if value == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return nil
	}
	return &t
}

func timeJSON(t *time.Time) map[string]any {
	if t == nil {
		return map[string]any{"value": nil, "timestamp": nil}
	}
	return map[string]any{
		"value":     t.Format("2006-01-02 15:04:05"),
		"timestamp": t.Unix(),
	}
}

func intPtrValue(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableEstablishedInt(state string, v *int, highlighted bool) any {
	if highlighted || state != "Established" || v == nil {
		return nil
	}
	return *v
}

func nullableInt(v int, highlighted bool) any {
	if highlighted {
		return nil
	}
	return v
}
