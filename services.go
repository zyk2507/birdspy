package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type App struct {
	cfg       *Config
	cache     *Cache
	reader    BirdReader
	serverSvc *RouteServerService
	symbolSvc *SymbolService
	peerSvc   *PeerService
	bgpSvc    *BgpProtocolService
	bfdSvc    *BfdSessionService
	routeSvc  *RouteTableService
	assets    *AssetManager
}

func NewApp(cfg *Config) (*App, error) {
	cache := NewCache(
		time.Duration(cfg.CacheTTLSeconds)*time.Second,
		cfg.CacheMaxEntries,
		time.Duration(cfg.CacheCleanupIntervalSeconds)*time.Second,
	)
	var reader BirdReader
	if cfg.Reader.Mode == "sample" {
		reader = NewSampleBirdReader(cfg.Reader.TinySamples)
	} else {
		reader = NewSocketBirdReader(cfg)
	}

	serverParser := NewRouteServerParser()
	symbolParser := NewSymbolParser()
	bgpParser := NewBgpProtocolParser()
	routeParser := NewRouteTableParser(cfg)
	bfdParser := NewBfdSessionParser()

	app := &App{cfg: cfg, cache: cache, reader: reader}
	app.serverSvc = &RouteServerService{cfg: cfg, reader: reader, parser: serverParser, cache: cache}
	app.bgpSvc = &BgpProtocolService{reader: reader, parser: bgpParser, routeParser: routeParser, cache: cache}
	app.peerSvc = &PeerService{cfg: cfg, bgpSvc: app.bgpSvc, cache: cache}
	app.symbolSvc = &SymbolService{reader: reader, parser: symbolParser, peerSvc: app.peerSvc, cache: cache}
	app.routeSvc = &RouteTableService{reader: reader, parser: routeParser, peerSvc: app.peerSvc, cache: cache}
	app.bgpSvc.routeSvc = app.routeSvc
	app.bfdSvc = &BfdSessionService{reader: reader, parser: bfdParser, peerSvc: app.peerSvc, cache: cache}
	assets, err := NewAssetManager()
	if err != nil {
		return nil, err
	}
	app.assets = assets
	return app, nil
}

func (a *App) Close() {
	if a.cache != nil {
		a.cache.Close()
	}
}

type RouteServerService struct {
	cfg    *Config
	reader BirdReader
	parser *RouteServerParser
	cache  *Cache
}

func (s *RouteServerService) Servers() (map[string]RouteServer, error) {
	value, err := s.cache.Get("servers", func() (any, error) {
		createdAt := time.Now()
		servers := map[string]RouteServer{}
		for _, id := range s.cfg.sortedServerIDs() {
			serverCfg := s.cfg.Servers[id]
			server := RouteServer{
				ID:          id,
				Name:        serverCfg.Name,
				Description: serverCfg.Description,
				Socket:      serverCfg.Socket,
				VersionMask: serverCfg.VersionMask,
				RouterID:    "unknown",
				CreatedAt:   createdAt,
			}
			data, err := s.reader.GetStatus(server)
			if err == nil {
				s.parser.Update(&server, data)
			} else {
				server.Message = err.Error()
			}
			servers[id] = server
			s.cache.Set(fmt.Sprintf("server-%s", id), server)
		}
		return servers, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[string]RouteServer), nil
}

func (s *RouteServerService) Server(id string) (RouteServer, error) {
	servers, err := s.Servers()
	if err != nil {
		return RouteServer{}, err
	}
	server, ok := servers[id]
	if !ok {
		return RouteServer{}, fmt.Errorf("route server %s does not exist", id)
	}
	return server, nil
}

func (s *RouteServerService) Protocol(server RouteServer, id string, symbols *SymbolService) (Symbol, error) {
	return symbols.Protocol(server, id)
}

func (s *RouteServerService) Table(server RouteServer, id string, symbols *SymbolService) (Symbol, error) {
	return symbols.Table(server, id)
}

type SymbolService struct {
	reader  BirdReader
	parser  *SymbolParser
	peerSvc *PeerService
	cache   *Cache
}

func (s *SymbolService) Symbols(server RouteServer) (map[string]map[string]Symbol, error) {
	key := fmt.Sprintf("%s-symbols", server.ID)
	value, err := s.cache.Get(key, func() (any, error) {
		data, err := s.reader.GetSymbols(server)
		if err != nil {
			return nil, err
		}
		parsed := s.parser.Symbols(data)
		protocols := map[string]Symbol{}
		tables := map[string]Symbol{}

		for _, symbol := range parsed["protocol"] {
			if strings.HasPrefix(symbol.ID, "R4_") || strings.HasPrefix(symbol.ID, "R6_") {
				symbol.Peer = s.peerSvc.PeerByProtocol(server, symbol.ID)
				protocols[symbol.ID] = symbol
			}
		}

		for _, symbol := range parsed["table"] {
			if isRouteTable(symbol.ID) {
				symbol.Peer = s.peerSvc.PeerByTable(server, symbol.ID)
				tables[symbol.ID] = symbol
			}
		}

		return map[string]map[string]Symbol{
			"protocol": protocols,
			"table":    tables,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[string]map[string]Symbol), nil
}

func (s *SymbolService) Protocols(server RouteServer) (map[string]Symbol, error) {
	symbols, err := s.Symbols(server)
	if err != nil {
		return nil, err
	}
	return symbols["protocol"], nil
}

func (s *SymbolService) Tables(server RouteServer) (map[string]Symbol, error) {
	symbols, err := s.Symbols(server)
	if err != nil {
		return nil, err
	}
	return symbols["table"], nil
}

func (s *SymbolService) Protocol(server RouteServer, id string) (Symbol, error) {
	protocols, err := s.Protocols(server)
	if err != nil {
		return Symbol{}, err
	}
	symbol, ok := protocols[id]
	if !ok {
		return Symbol{}, fmt.Errorf("route protocol %s does not exist", id)
	}
	return symbol, nil
}

func (s *SymbolService) Table(server RouteServer, id string) (Symbol, error) {
	tables, err := s.Tables(server)
	if err != nil {
		return Symbol{}, err
	}
	symbol, ok := tables[id]
	if !ok {
		return Symbol{}, fmt.Errorf("route table %s does not exist", id)
	}
	return symbol, nil
}

type PeerService struct {
	cfg    *Config
	bgpSvc *BgpProtocolService
	cache  *Cache
}

func (s *PeerService) Peers(server RouteServer) ([]Peer, error) {
	key := fmt.Sprintf("%s-peers", server.ID)
	value, err := s.cache.Get(key, func() (any, error) {
		protocols, err := s.bgpSvc.Protocols(server)
		if err != nil {
			return nil, err
		}
		peers := []Peer{}
		blackHoleIPs := make([]string, 0, len(s.cfg.BlackHoles))
		for ip := range s.cfg.BlackHoles {
			blackHoleIPs = append(blackHoleIPs, ip)
		}
		sort.Strings(blackHoleIPs)
		for _, ip := range blackHoleIPs {
			peer := defaultPeer()
			peer.Name = s.cfg.BlackHoles[ip]
			peer.IP = ip
			peers = append(peers, peer)
		}
		for _, protocol := range protocols.Data {
			peer := defaultPeer()
			peer.Name = protocol.PeerName()
			peer.Table = protocol.Table
			peer.ASN = protocol.ASN
			peer.IP = protocol.NeighborAddress
			peer.Description = protocol.FormattedDescription()
			peers = append(peers, peer)
		}
		return peers, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]Peer), nil
}

func (s *PeerService) PeerByIP(server RouteServer, ip string) Peer {
	peers, err := s.Peers(server)
	if err == nil {
		for _, peer := range peers {
			if peer.IP == ip {
				return peer
			}
		}
	}
	peer := defaultPeer()
	peer.IP = ip
	return peer
}

func (s *PeerService) PeerByProtocol(server RouteServer, protocol string) Peer {
	table := protocol
	if len(protocol) > 1 {
		table = "T" + protocol[1:]
	}
	return s.PeerByTable(server, table)
}

func (s *PeerService) PeerByTable(server RouteServer, table string) Peer {
	peers, err := s.Peers(server)
	if err == nil {
		for _, peer := range peers {
			if peer.Table == table {
				return peer
			}
		}
	}
	peer := defaultPeer()
	peer.Table = table
	return peer
}

func (s *PeerService) NamesByIP(server RouteServer) map[string]string {
	results := map[string]string{}
	peers, err := s.Peers(server)
	if err != nil {
		return results
	}
	for _, peer := range peers {
		results[peer.IP] = peer.DisplayName()
	}
	return results
}

func (s *PeerService) ASNsByIP(server RouteServer) map[string]any {
	results := map[string]any{}
	peers, err := s.Peers(server)
	if err != nil {
		return results
	}
	for _, peer := range peers {
		results[peer.IP] = peer.ASN
	}
	return results
}

type ProtocolResults struct {
	Data      []*BgpProtocol
	Timestamp int64
}

type BgpProtocolService struct {
	reader      BirdReader
	parser      *BgpProtocolParser
	routeParser *RouteTableParser
	routeSvc    *RouteTableService
	cache       *Cache
}

func (s *BgpProtocolService) Protocols(server RouteServer) (ProtocolResults, error) {
	key := fmt.Sprintf("%s-bgp-protocols", server.ID)
	value, err := s.cache.Get(key, func() (any, error) {
		createdAt := time.Now()
		data, err := s.reader.GetBgpProtocols(server, false)
		if err != nil {
			return nil, err
		}
		protocols := s.parser.Protocols(data)
		selectedCounts := s.routeSvc.CountsForFiltered(server)
		invalidCounts := s.routeSvc.CountsForInvalid(server)
		for _, protocol := range protocols {
			protocol.CreatedAt = createdAt
			protocol.SelectedRoutes = selectedCounts[protocol.Table]
			protocol.InvalidRoutes = invalidCounts[protocol.Table]
			s.cache.Set(protocol.ID, *protocol)
		}
		return ProtocolResults{Data: protocols, Timestamp: createdAt.Unix()}, nil
	})
	if err != nil {
		return ProtocolResults{}, err
	}
	return value.(ProtocolResults), nil
}

func (s *BgpProtocolService) JSON(server RouteServer) (jsonResponse, error) {
	results, err := s.Protocols(server)
	if err != nil {
		return jsonResponse{}, err
	}
	data := make([]map[string]any, 0, len(results.Data))
	for _, protocol := range results.Data {
		data = append(data, protocol.ToJSON())
	}
	return jsonResponse{Data: data, Timestamp: results.Timestamp}, nil
}

func (s *BgpProtocolService) Detail(id string) jsonResponse {
	if value, ok := s.cache.Peek(id); ok {
		if protocol, ok := value.(BgpProtocol); ok {
			return jsonResponse{Data: map[string]any{"blob": protocol.Blob}, Timestamp: protocol.CreatedAt.Unix()}
		}
	}
	return jsonResponse{Data: map[string]any{"blob": "Cache expired. Try reload page."}, Timestamp: time.Now().Unix()}
}

type BfdSessionService struct {
	reader  BirdReader
	parser  *BfdSessionParser
	peerSvc *PeerService
	cache   *Cache
}

func (s *BfdSessionService) Sessions(server RouteServer) ([]BfdSession, int64, error) {
	key := fmt.Sprintf("%s-bfd-sessions", server.ID)
	value, err := s.cache.Get(key, func() (any, error) {
		createdAt := time.Now()
		data, err := s.reader.GetBfdSessions(server)
		if err != nil {
			return nil, err
		}
		sessions := s.parser.Sessions(data)
		for i := range sessions {
			sessions[i].Peer = s.peerSvc.PeerByIP(server, sessions[i].IPAddress)
		}
		return bfdResults{Data: sessions, Timestamp: createdAt.Unix()}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	results := value.(bfdResults)
	return results.Data, results.Timestamp, nil
}

func (s *BfdSessionService) JSON(server RouteServer) (jsonResponse, error) {
	sessions, timestamp, err := s.Sessions(server)
	if err != nil {
		return jsonResponse{}, err
	}
	data := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		data = append(data, session.ToJSON())
	}
	return jsonResponse{Data: data, Timestamp: timestamp}, nil
}

type bfdResults struct {
	Data      []BfdSession
	Timestamp int64
}

type routeTableRefs struct {
	Data      map[string]string
	Timestamp int64
}

type routeResults struct {
	Data      []*Route
	Timestamp int64
}

type RouteTableService struct {
	reader  BirdReader
	parser  *RouteTableParser
	peerSvc *PeerService
	cache   *Cache
}

func (s *RouteTableService) TableRoutes(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-table-routes-%s", server.ID, params.Table)
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetTableRoutes(server, params)
	})
}

func (s *RouteTableService) TableRoutesForPrefix(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-table-routes-%s-%s", server.ID, params.Table, params.PrefixKey())
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetTableRoutesForPrefix(server, params)
	})
}

func (s *RouteTableService) TableRoutesByCommunity(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-table-%s-community-routes-%s", server.ID, params.Table, params.CommunitiesKey())
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetTableRoutesFilteredByCommunity(server, params)
	})
}

func (s *RouteTableService) ImportedRoutes(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-imported-routes-%s", server.ID, params.Protocol)
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetImportedRoutes(server, params)
	})
}

func (s *RouteTableService) ImportedRoutesForPrefix(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-imported-routes-%s-%s", server.ID, params.Protocol, params.PrefixKey())
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetImportedRoutesForPrefix(server, params)
	})
}

func (s *RouteTableService) ExportedRoutes(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-exported-routes-%s", server.ID, params.Export)
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetExportedRoutes(server, params)
	})
}

func (s *RouteTableService) ExportedRoutesForPrefix(server RouteServer, params CommandParameters) (routeResults, error) {
	key := fmt.Sprintf("%s-exported-routes-%s-%s", server.ID, params.Export, params.PrefixKey())
	return s.cachedRoutes(key, server, func() (string, error) {
		return s.reader.GetExportedRoutesForPrefix(server, params)
	})
}

func (s *RouteTableService) FilteredRoutes(server RouteServer) (routeResults, error) {
	return s.cachedRoutes(fmt.Sprintf("%s-filtered-routes", server.ID), server, func() (string, error) {
		return s.reader.GetFilteredRoutes(server, false)
	})
}

func (s *RouteTableService) TableFilteredRoutes(server RouteServer, table string) (routeResults, error) {
	results, err := s.FilteredRoutes(server)
	if err != nil {
		return routeResults{}, err
	}
	return narrowRouteResults(results, table)
}

func (s *RouteTableService) InvalidRoutes(server RouteServer) (routeResults, error) {
	return s.cachedRoutes(fmt.Sprintf("%s-invalid-routes", server.ID), server, func() (string, error) {
		return s.reader.GetInvalidRoutes(server, false)
	})
}

func (s *RouteTableService) TableInvalidRoutes(server RouteServer, table string) (routeResults, error) {
	results, err := s.InvalidRoutes(server)
	if err != nil {
		return routeResults{}, err
	}
	return narrowRouteResults(results, table)
}

func narrowRouteResults(results routeResults, table string) (routeResults, error) {
	filtered := []*Route{}
	for _, route := range results.Data {
		if route.TableName == table {
			filtered = append(filtered, route)
		}
	}
	if len(filtered) == 0 {
		return routeResults{}, fmt.Errorf("route table %s does not exist in dataset", table)
	}
	return routeResults{Data: filtered, Timestamp: results.Timestamp}, nil
}

func (s *RouteTableService) cachedRoutes(key string, server RouteServer, load func() (string, error)) (routeResults, error) {
	value, err := s.cache.Get(key, func() (any, error) {
		data, err := load()
		if err != nil {
			return nil, err
		}
		refs := s.createRouteTables(server, data)
		return refs, nil
	})
	if err != nil {
		return routeResults{}, err
	}
	return s.cachedRouteValues(value.(routeTableRefs))
}

func (s *RouteTableService) createRouteTables(server RouteServer, data string) routeTableRefs {
	createdAt := time.Now()
	neighborNames := s.peerSvc.NamesByIP(server)
	neighborASNs := s.peerSvc.ASNsByIP(server)
	refs := routeTableRefs{Data: map[string]string{}, Timestamp: createdAt.Unix()}

	for _, table := range s.parser.RouteTables(data) {
		if !isRouteTable(table.Name) {
			continue
		}
		peer := s.peerSvc.PeerByTable(server, table.Name)
		table.Peer = peer
		table.CreatedAt = createdAt
		table.Routes = map[string]*Route{}

		for _, route := range s.parser.Routes(table.Blob) {
			if name, ok := neighborNames[route.NextHop]; ok {
				route.NeighborName = name
			}
			if asn, ok := neighborASNs[route.NextHop]; ok {
				route.NeighborASN = asn
			}
			route.TableName = table.Name
			route.TableID = table.ID
			route.PeerName = peer.DisplayName()
			route.PeerASN = peer.ASN
			route.Description = peer.DisplayDescription()
			table.Routes[route.ID] = route
		}

		table.Blob = ""
		s.cache.Set(table.ID, *table)
		refs.Data[table.Name] = table.ID
	}

	return refs
}

func (s *RouteTableService) cachedRouteValues(refs routeTableRefs) (routeResults, error) {
	routes := []*Route{}
	tableNames := make([]string, 0, len(refs.Data))
	for name := range refs.Data {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)
	for _, name := range tableNames {
		id := refs.Data[name]
		value, ok := s.cache.Peek(id)
		if !ok {
			continue
		}
		table, ok := value.(RouteTable)
		if !ok {
			continue
		}
		routeIDs := make([]string, 0, len(table.Routes))
		for id := range table.Routes {
			routeIDs = append(routeIDs, id)
		}
		sort.Strings(routeIDs)
		for _, id := range routeIDs {
			routes = append(routes, table.Routes[id])
		}
	}
	return routeResults{Data: routes, Timestamp: refs.Timestamp}, nil
}

func (s *RouteTableService) JSON(results routeResults) jsonResponse {
	data := make([]map[string]any, 0, len(results.Data))
	for _, route := range results.Data {
		data = append(data, route.ToJSON())
	}
	return jsonResponse{Data: data, Timestamp: results.Timestamp}
}

func (s *RouteTableService) Detail(tableID, routeID string) jsonResponse {
	if value, ok := s.cache.Peek(tableID); ok {
		if table, ok := value.(RouteTable); ok {
			if route, ok := table.Routes[routeID]; ok {
				return jsonResponse{Data: map[string]any{"blob": route.Blob}, Timestamp: table.CreatedAt.Unix()}
			}
		}
	}
	return jsonResponse{Data: map[string]any{"blob": "Cache expired. Try reload page."}, Timestamp: time.Now().Unix()}
}

func (s *RouteTableService) CountsForFiltered(server RouteServer) map[string]int {
	return s.counts(fmt.Sprintf("%s-filtered-counts", server.ID), func() (string, error) {
		return s.reader.GetFilteredRoutes(server, false)
	})
}

func (s *RouteTableService) CountsForInvalid(server RouteServer) map[string]int {
	return s.counts(fmt.Sprintf("%s-invalid-counts", server.ID), func() (string, error) {
		return s.reader.GetInvalidRoutes(server, false)
	})
}

func (s *RouteTableService) counts(key string, load func() (string, error)) map[string]int {
	value, err := s.cache.Get(key, func() (any, error) {
		data, err := load()
		if err != nil {
			return map[string]int{}, nil
		}
		return s.parser.CountByTable(data), nil
	})
	if err != nil {
		return map[string]int{}
	}
	return value.(map[string]int)
}

func communitySelectorsFromQuery(c, lgc, ec string) ([]CommunitySelector, error) {
	results := []CommunitySelector{}
	for _, value := range commaValues(c) {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid BGP community %q", value)
		}
		results = append(results, CommunitySelector{Kind: "community", Values: parts})
	}
	for _, value := range commaValues(lgc) {
		parts := strings.Split(value, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid BGP large community %q", value)
		}
		results = append(results, CommunitySelector{Kind: "large", Values: parts})
	}
	for _, value := range commaValues(ec) {
		parts := strings.Split(value, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid BGP extended community %q", value)
		}
		results = append(results, CommunitySelector{Kind: "extended", Values: parts})
	}
	if len(results) == 0 {
		return nil, errors.New("BGP communities not found")
	}
	return results, nil
}

func commaValues(value string) []string {
	values := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func sourceValue(kind, id string) string {
	return kind + ":" + id
}

func parseSourceValue(raw string) (string, string, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid source %q", raw)
	}
	switch parts[0] {
	case "table", "imported_protocol", "exported_protocol":
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid source type %q", parts[0])
	}
}
