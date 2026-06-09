package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
	"time"
)

type BirdReader interface {
	GetStatus(server RouteServer) (string, error)
	GetSymbols(server RouteServer) (string, error)
	GetBfdSessions(server RouteServer) (string, error)
	GetBgpProtocols(server RouteServer, count bool) (string, error)
	GetRoutes(server RouteServer, count bool) (string, error)
	GetInvalidRoutes(server RouteServer, count bool) (string, error)
	GetFilteredRoutes(server RouteServer, count bool) (string, error)
	GetTableRoutes(server RouteServer, params CommandParameters) (string, error)
	GetImportedRoutes(server RouteServer, params CommandParameters) (string, error)
	GetExportedRoutes(server RouteServer, params CommandParameters) (string, error)
	GetTableRoutesForPrefix(server RouteServer, params CommandParameters) (string, error)
	GetImportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error)
	GetExportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error)
	GetTableRoutesFilteredByCommunity(server RouteServer, params CommandParameters) (string, error)
}

type SocketBirdReader struct {
	timeout time.Duration
	cfg     *Config
}

func NewSocketBirdReader(cfg *Config) *SocketBirdReader {
	return &SocketBirdReader{
		timeout: time.Duration(cfg.Reader.CommandTimeoutSec) * time.Second,
		cfg:     cfg,
	}
}

func (r *SocketBirdReader) commandOutput(server RouteServer, cmd string) (string, error) {
	if server.Socket == "" {
		return "", fmt.Errorf("server %s has no control socket configured", server.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", server.Socket)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	reader := bufio.NewReader(conn)
	var out strings.Builder

	greeting, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	appendBirdLine(&out, greeting)

	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return "", err
	}

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			done := appendBirdLine(&out, line)
			if done {
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
	}

	return out.String(), nil
}

func appendBirdLine(out *strings.Builder, raw string) bool {
	line := strings.TrimRight(raw, "\r\n")
	if len(line) >= 5 && isDigits(line[:4]) && (line[4] == ' ' || line[4] == '-') {
		code := line[:4]
		if code == "0000" {
			return true
		}
		line = line[5:]
	}
	if strings.TrimSpace(line) != "" {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return false
}

func isDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (r *SocketBirdReader) GetStatus(server RouteServer) (string, error) {
	return r.commandOutput(server, showStatus())
}

func (r *SocketBirdReader) GetSymbols(server RouteServer) (string, error) {
	return r.commandOutput(server, showSymbols())
}

func (r *SocketBirdReader) GetBfdSessions(server RouteServer) (string, error) {
	return r.commandOutput(server, showBfdSessions())
}

func (r *SocketBirdReader) GetBgpProtocols(server RouteServer, count bool) (string, error) {
	return r.commandOutput(server, showProtocols(count))
}

func (r *SocketBirdReader) GetRoutes(server RouteServer, count bool) (string, error) {
	return r.commandOutput(server, showRoute(count))
}

func (r *SocketBirdReader) GetInvalidRoutes(server RouteServer, count bool) (string, error) {
	params := filteredParameters(r.cfg.Invalid, count)
	return r.commandOutput(server, showRouteTableFilteredByCommunity(params))
}

func (r *SocketBirdReader) GetFilteredRoutes(server RouteServer, count bool) (string, error) {
	params := filteredParameters(r.cfg.Filtered, count)
	return r.commandOutput(server, showRouteTableFilteredByCommunity(params))
}

func (r *SocketBirdReader) GetTableRoutes(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteTable(params))
}

func (r *SocketBirdReader) GetImportedRoutes(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteProtocol(params))
}

func (r *SocketBirdReader) GetExportedRoutes(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteExport(params))
}

func (r *SocketBirdReader) GetTableRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteForPrefixAndTable(params))
}

func (r *SocketBirdReader) GetImportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteForPrefixAndProtocol(params))
}

func (r *SocketBirdReader) GetExportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteForPrefixAndExport(params))
}

func (r *SocketBirdReader) GetTableRoutesFilteredByCommunity(server RouteServer, params CommandParameters) (string, error) {
	return r.commandOutput(server, showRouteTableFilteredByCommunity(params))
}

type SampleBirdReader struct {
	tiny bool
}

func NewSampleBirdReader(tiny bool) *SampleBirdReader {
	return &SampleBirdReader{tiny: tiny}
}

func (r *SampleBirdReader) sampleFile(server RouteServer, filename string) (string, error) {
	version := server.VersionMask
	if version == "" {
		version = "v-2.x"
	}
	raw, err := embeddedFiles.ReadFile(path.Join("files/bird-samples", version, filename))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (r *SampleBirdReader) GetStatus(server RouteServer) (string, error) {
	return r.sampleFile(server, "show_status.txt")
}

func (r *SampleBirdReader) GetSymbols(server RouteServer) (string, error) {
	return r.sampleFile(server, "show_symbols.txt")
}

func (r *SampleBirdReader) GetBfdSessions(server RouteServer) (string, error) {
	return r.sampleFile(server, "show_bfd_sessions.txt")
}

func (r *SampleBirdReader) GetBgpProtocols(server RouteServer, count bool) (string, error) {
	if count {
		return r.sampleFile(server, "show_protocols_count.txt")
	}
	return r.sampleFile(server, "show_protocols_all.txt")
}

func (r *SampleBirdReader) GetRoutes(server RouteServer, count bool) (string, error) {
	if count {
		return r.sampleFile(server, "show_route_table_all_count.txt")
	}
	return r.sampleFile(server, "show_route_table_all.txt")
}

func (r *SampleBirdReader) GetInvalidRoutes(server RouteServer, count bool) (string, error) {
	if count {
		return r.sampleFile(server, "show_route_table_all_invalid_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_table_all_invalid_all.txt")
}

func (r *SampleBirdReader) GetFilteredRoutes(server RouteServer, count bool) (string, error) {
	if count {
		return r.sampleFile(server, "show_route_table_all_filtered_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_table_all_filtered_all.txt")
}

func (r *SampleBirdReader) GetTableRoutes(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_table_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_table_all.txt")
}

func (r *SampleBirdReader) GetImportedRoutes(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_protocol_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_protocol_all.txt")
}

func (r *SampleBirdReader) GetExportedRoutes(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_export_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_export_all.txt")
}

func (r *SampleBirdReader) GetTableRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_for_net_table_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_for_net_table_all.txt")
}

func (r *SampleBirdReader) GetImportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_for_net_protocol_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_for_net_protocol_all.txt")
}

func (r *SampleBirdReader) GetExportedRoutesForPrefix(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_for_net_export_count.txt")
	}
	if r.tiny {
		return r.sampleFile(server, "show_route_table_all-tiny.txt")
	}
	return r.sampleFile(server, "show_route_for_net_export_all.txt")
}

func (r *SampleBirdReader) GetTableRoutesFilteredByCommunity(server RouteServer, params CommandParameters) (string, error) {
	if params.Count {
		return r.sampleFile(server, "show_route_table_table_filtered_count.txt")
	}
	return r.sampleFile(server, "show_route_table_table_filtered_all.txt")
}

type CommandParameters struct {
	Table                   string
	Protocol                string
	Export                  string
	Prefix                  string
	BgpCommunities          []CommunitySelector
	BgpCommunitiesCondition string
	Count                   bool
}

func NewCommandParameters() CommandParameters {
	return CommandParameters{
		Table:                   "all",
		BgpCommunitiesCondition: "||",
	}
}

func (p CommandParameters) Suffix() string {
	if p.Count {
		return "count"
	}
	return "all"
}

func (p CommandParameters) CommunitiesKey() string {
	values := make([]string, 0, len(p.BgpCommunities))
	for _, community := range p.BgpCommunities {
		values = append(values, community.Raw())
	}
	return stableKey(strings.Join(values, "-"))
}

func (p CommandParameters) PrefixKey() string {
	return stableKey(p.Prefix)
}

type CommunitySelector struct {
	Kind   string
	Values []string
}

func (c CommunitySelector) FilterValue() string {
	return strings.Join(c.Values, ":")
}

func (c CommunitySelector) Raw() string {
	switch c.Kind {
	case "large", "extended":
		return fmt.Sprintf("(%s, %s, %s)", valueAt(c.Values, 0), valueAt(c.Values, 1), valueAt(c.Values, 2))
	default:
		return fmt.Sprintf("(%s,%s)", valueAt(c.Values, 0), valueAt(c.Values, 1))
	}
}

func (c CommunitySelector) CommandExpr() string {
	switch c.Kind {
	case "large":
		return fmt.Sprintf("(bgp_large_community ~ [(%s, %s, %s)])", valueAt(c.Values, 0), valueAt(c.Values, 1), valueAt(c.Values, 2))
	case "extended":
		return fmt.Sprintf("(bgp_ext_community ~ [(%s, %s, %s)])", valueAt(c.Values, 0), valueAt(c.Values, 1), valueAt(c.Values, 2))
	default:
		return fmt.Sprintf("(bgp_community ~ [(%s, %s)])", valueAt(c.Values, 0), valueAt(c.Values, 1))
	}
}

func valueAt(values []string, idx int) string {
	if idx >= len(values) {
		return ""
	}
	return values[idx]
}

func filteredParameters(filters CommunityFilters, count bool) CommandParameters {
	params := NewCommandParameters()
	params.Count = count
	for _, values := range filters.Communities {
		params.BgpCommunities = append(params.BgpCommunities, CommunitySelector{Kind: "community", Values: values})
	}
	for _, values := range filters.LargeCommunities {
		params.BgpCommunities = append(params.BgpCommunities, CommunitySelector{Kind: "large", Values: values})
	}
	for _, values := range filters.ExtendedCommunities {
		params.BgpCommunities = append(params.BgpCommunities, CommunitySelector{Kind: "extended", Values: values})
	}
	return params
}

func showStatus() string {
	return "show status"
}

func showSymbols() string {
	return "show symbols"
}

func showProtocols(count bool) string {
	if count {
		return "show protocols count"
	}
	return "show protocols all"
}

func showBfdSessions() string {
	return "show bfd sessions"
}

func showRoute(count bool) string {
	if count {
		return "show route count"
	}
	return "show route all"
}

func showRouteTable(params CommandParameters) string {
	return fmt.Sprintf("show route table %s %s", params.Table, params.Suffix())
}

func showRouteProtocol(params CommandParameters) string {
	return fmt.Sprintf("show route protocol %s %s", params.Protocol, params.Suffix())
}

func showRouteExport(params CommandParameters) string {
	return fmt.Sprintf("show route export %s %s", params.Export, params.Suffix())
}

func showRouteForPrefixAndTable(params CommandParameters) string {
	return fmt.Sprintf("show route for %s table %s %s", params.Prefix, params.Table, params.Suffix())
}

func showRouteForPrefixAndProtocol(params CommandParameters) string {
	return fmt.Sprintf("show route for %s protocol %s %s", params.Prefix, params.Protocol, params.Suffix())
}

func showRouteForPrefixAndExport(params CommandParameters) string {
	return fmt.Sprintf("show route for %s export %s %s", params.Prefix, params.Export, params.Suffix())
}

func showRouteTableFilteredByCommunity(params CommandParameters) string {
	expressions := make([]string, 0, len(params.BgpCommunities))
	for _, community := range params.BgpCommunities {
		expressions = append(expressions, community.CommandExpr())
	}
	condition := params.BgpCommunitiesCondition
	if condition == "" {
		condition = "||"
	}
	if len(expressions) == 0 {
		expressions = append(expressions, "false")
	}
	return fmt.Sprintf(
		"show route table %s filter { if %s then accept; reject; } %s",
		params.Table,
		strings.Join(expressions, " "+condition+" "),
		params.Suffix(),
	)
}
