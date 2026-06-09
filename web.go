package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type PageData struct {
	Route                 string
	Title                 string
	Subtitle              string
	HeadingText           string
	ContentTemplate       string
	Site                  SiteConfig
	Organization          OrganizationConfig
	Year                  int
	ServerID              string
	Servers               []RouteServer
	CurrentServer         RouteServer
	ShowServerDropdown    bool
	RequestPath           string
	CSS                   []string
	JS                    []string
	DataURL               string
	Grouped               bool
	JustInvalid           bool
	Tables                []Symbol
	Protocols             []Symbol
	SelectedTable         string
	NetworkSources        []SelectOption
	CommunityOptions      []SelectOption
	RouteCommunityOptions []SelectOption
	Flags                 []Flag
}

type SelectOption struct {
	Value      string
	Label      string
	Subtext    string
	Group      string
	Content    template.HTML
	GroupStart bool
	GroupEnd   bool
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.homePage)
	mux.HandleFunc("GET /api/protocol/detail", a.protocolDetailAPI)
	mux.HandleFunc("GET /api/route/detail", a.routeDetailAPI)

	mux.HandleFunc("GET /{server}/bfd/sessions", a.bfdSessionsPage)
	mux.HandleFunc("GET /api/{server}/bfd/sessions", a.bfdSessionsAPI)
	mux.HandleFunc("GET /{server}/bgp/protocols", a.bgpProtocolsPage)
	mux.HandleFunc("GET /api/{server}/bgp/protocols", a.bgpProtocolsAPI)
	mux.HandleFunc("GET /{server}/community/lookup", a.communityLookupPage)
	mux.HandleFunc("POST /{server}/community/lookup", a.communityLookupPost)
	mux.HandleFunc("GET /{server}/network/lookup", a.networkLookupPage)
	mux.HandleFunc("POST /{server}/network/lookup", a.networkLookupPost)

	mux.HandleFunc("GET /{server}/invalid-routes", a.invalidRoutesPage)
	mux.HandleFunc("GET /api/{server}/invalid-routes", a.invalidRoutesAPI)
	mux.HandleFunc("GET /{server}/filtered-routes", a.filteredRoutesPage)
	mux.HandleFunc("GET /api/{server}/filtered-routes", a.filteredRoutesAPI)

	mux.HandleFunc("GET /{server}/table/{table}/routes", a.tableRoutesPage)
	mux.HandleFunc("GET /api/{server}/table/{table}/routes", a.tableRoutesAPI)
	mux.HandleFunc("GET /{server}/table/{table}/prefix-routes", a.prefixTableRoutesPage)
	mux.HandleFunc("GET /api/{server}/table/{table}/prefix-routes", a.prefixTableRoutesAPI)
	mux.HandleFunc("GET /{server}/table/{table}/community-routes", a.communityTableRoutesPage)
	mux.HandleFunc("GET /api/{server}/table/{table}/community-routes", a.communityTableRoutesAPI)
	mux.HandleFunc("GET /{server}/table/{table}/invalid-routes", a.tableInvalidRoutesPage)
	mux.HandleFunc("GET /api/{server}/table/{table}/invalid-routes", a.tableInvalidRoutesAPI)
	mux.HandleFunc("GET /{server}/table/{table}/filtered-routes", a.tableFilteredRoutesPage)
	mux.HandleFunc("GET /api/{server}/table/{table}/filtered-routes", a.tableFilteredRoutesAPI)

	mux.HandleFunc("GET /{server}/protocol/{protocol}/routes", a.importedRoutesPage)
	mux.HandleFunc("GET /api/{server}/protocol/{protocol}/routes", a.importedRoutesAPI)
	mux.HandleFunc("GET /{server}/protocol/{protocol}/prefix-routes", a.prefixImportedRoutesPage)
	mux.HandleFunc("GET /api/{server}/protocol/{protocol}/prefix-routes", a.prefixImportedRoutesAPI)
	mux.HandleFunc("GET /{server}/export/{protocol}/routes", a.exportedRoutesPage)
	mux.HandleFunc("GET /api/{server}/export/{protocol}/routes", a.exportedRoutesAPI)
	mux.HandleFunc("GET /{server}/export/{protocol}/prefix-routes", a.prefixExportedRoutesPage)
	mux.HandleFunc("GET /api/{server}/export/{protocol}/prefix-routes", a.prefixExportedRoutesAPI)
	static := http.FileServer(http.FS(a.assets.publicFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/build/") {
			static.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) basePage(r *http.Request, route, title, subtitle string, entries ...string) (PageData, error) {
	serverID := r.PathValue("server")
	if serverID == "" {
		serverID = a.cfg.DefaultServer
	}
	serversMap, err := a.serverSvc.Servers()
	if err != nil {
		return PageData{}, err
	}
	servers := make([]RouteServer, 0, len(serversMap))
	for _, id := range a.cfg.sortedServerIDs() {
		servers = append(servers, serversMap[id])
	}
	current, ok := serversMap[serverID]
	if !ok {
		current = serversMap[a.cfg.DefaultServer]
		serverID = a.cfg.DefaultServer
	}
	allEntries := append([]string{"app"}, entries...)
	css, js := a.assets.Tags(allEntries...)
	return PageData{
		Route:              route,
		Title:              title,
		Subtitle:           subtitle,
		Site:               a.cfg.Site,
		Organization:       a.cfg.Organization,
		Year:               time.Now().Year(),
		ServerID:           serverID,
		Servers:            servers,
		CurrentServer:      current,
		ShowServerDropdown: len(servers) > 1 && route != "homepage",
		RequestPath:        r.URL.RequestURI(),
		CSS:                css,
		JS:                 js,
		Flags:              sortedFlags(a.cfg.Flags),
	}, nil
}

func (a *App) render(w http.ResponseWriter, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.assets.templates.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) homePage(w http.ResponseWriter, r *http.Request) {
	data, err := a.basePage(r, "homepage", "Homepage", a.cfg.Site.Title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "home"
	data.HeadingText = a.cfg.Site.Subtitle
	a.render(w, data)
}

func (a *App) bfdSessionsPage(w http.ResponseWriter, r *http.Request) {
	data, err := a.basePage(r, "bfd_sessions", "BFD Sessions Summary", serverSubtitle(a, r), "bfd_sessions")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "bfd_sessions"
	a.render(w, data)
}

func (a *App) bgpProtocolsPage(w http.ResponseWriter, r *http.Request) {
	data, err := a.basePage(r, "bgp_protocols", "BGP Protocols Summary", serverSubtitle(a, r), "bgp_protocols")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "bgp_protocols"
	a.render(w, data)
}

func (a *App) networkLookupPage(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, err := a.basePage(r, "network_lookup", "Network Prefix Lookup", server.Name, "network_lookup")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "network_lookup"
	data.NetworkSources = a.networkSources(server)
	a.render(w, data)
}

func (a *App) networkLookupPost(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	kind, id, err := parseSourceValue(r.Form.Get("source"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prefix, err := normalizePrefix(r.Form.Get("network"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	values := url.Values{"prefix": []string{prefix}}
	switch kind {
	case "imported_protocol":
		http.Redirect(w, r, pathWithQuery(fmt.Sprintf("/%s/protocol/%s/prefix-routes", server.ID, id), values), http.StatusSeeOther)
	case "exported_protocol":
		http.Redirect(w, r, pathWithQuery(fmt.Sprintf("/%s/export/%s/prefix-routes", server.ID, id), values), http.StatusSeeOther)
	default:
		http.Redirect(w, r, pathWithQuery(fmt.Sprintf("/%s/table/%s/prefix-routes", server.ID, id), values), http.StatusSeeOther)
	}
}

func (a *App) communityLookupPage(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, err := a.basePage(r, "community_lookup", "BGP Community Lookup", server.Name, "community_lookup")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "community_lookup"
	data.Tables = a.sortedTables(server)
	data.SelectedTable = r.URL.Query().Get("table")
	data.CommunityOptions = a.communityOptions()
	a.render(w, data)
}

func (a *App) communityLookupPost(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	table := r.Form.Get("table")
	values := url.Values{}
	c, lgc, ec := []string{}, []string{}, []string{}
	for _, raw := range r.Form["community"] {
		kind, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		switch kind {
		case "c":
			c = append(c, value)
		case "lgc":
			lgc = append(lgc, value)
		case "ec":
			ec = append(ec, value)
		}
	}
	if len(c) > 0 {
		values.Set("c", strings.Join(c, ","))
	}
	if len(lgc) > 0 {
		values.Set("lgc", strings.Join(lgc, ","))
	}
	if len(ec) > 0 {
		values.Set("ec", strings.Join(ec, ","))
	}
	http.Redirect(w, r, pathWithQuery(fmt.Sprintf("/%s/table/%s/community-routes", server.ID, table), values), http.StatusSeeOther)
}

func (a *App) routePage(w http.ResponseWriter, r *http.Request, route, title, heading, dataURL string, grouped, justInvalid bool) {
	data, err := a.basePage(r, route, title, serverSubtitle(a, r), "server_routes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ContentTemplate = "routes"
	data.HeadingText = heading
	data.DataURL = dataURL
	data.Grouped = grouped
	data.JustInvalid = justInvalid
	data.RouteCommunityOptions = a.routeCommunityOptions()
	a.render(w, data)
}

func (a *App) tableRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	heading := fmt.Sprintf("Routes from table: %s %s on server: %s", table.ID, table.Peer.DisplayName(), server.Name)
	a.routePage(w, r, "table_routes", "Table Routes", heading, fmt.Sprintf("/api/%s/table/%s/routes", server.ID, table.ID), false, false)
}

func (a *App) prefixTableRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	heading := fmt.Sprintf("Routes from table: %s %s on server: %s for network prefix: %s", table.ID, table.Peer.DisplayName(), server.Name, prefix)
	a.routePage(w, r, "prefix_table_routes", "Table Routes for Network Prefix", heading, fmt.Sprintf("/api/%s/table/%s/prefix-routes?prefix=%s", server.ID, table.ID, url.QueryEscape(prefix)), false, false)
}

func (a *App) communityTableRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	communities, err := communitySelectorsFromQuery(r.URL.Query().Get("c"), r.URL.Query().Get("lgc"), r.URL.Query().Get("ec"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw := []string{}
	for _, community := range communities {
		raw = append(raw, community.Raw())
	}
	heading := fmt.Sprintf("Routes from table: %s %s on server: %s with BGP community: %s", table.ID, table.Peer.DisplayName(), server.Name, strings.Join(raw, ", "))
	a.routePage(w, r, "community_table_routes", "Community Table Routes", heading, "/api"+r.URL.RequestURI(), false, false)
}

func (a *App) importedRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	heading := fmt.Sprintf("Routes imported from protocol: %s %s on server: %s", protocol.ID, protocol.Peer.DisplayName(), server.Name)
	a.routePage(w, r, "imported_routes", "Imported Routes", heading, fmt.Sprintf("/api/%s/protocol/%s/routes", server.ID, protocol.ID), false, false)
}

func (a *App) prefixImportedRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	heading := fmt.Sprintf("Routes imported from protocol: %s %s on server: %s for network prefix: %s", protocol.ID, protocol.Peer.DisplayName(), server.Name, prefix)
	a.routePage(w, r, "prefix_imported_routes", "Imported Routes for Network Prefix", heading, fmt.Sprintf("/api/%s/protocol/%s/prefix-routes?prefix=%s", server.ID, protocol.ID, url.QueryEscape(prefix)), false, false)
}

func (a *App) exportedRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	heading := fmt.Sprintf("Routes exported to protocol: %s %s on server: %s", protocol.ID, protocol.Peer.DisplayName(), server.Name)
	a.routePage(w, r, "exported_routes", "Exported Routes", heading, fmt.Sprintf("/api/%s/export/%s/routes", server.ID, protocol.ID), false, false)
}

func (a *App) prefixExportedRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	heading := fmt.Sprintf("Routes exported to protocol: %s %s on server: %s for network prefix: %s", protocol.ID, protocol.Peer.DisplayName(), server.Name, prefix)
	a.routePage(w, r, "prefix_exported_routes", "Exported Routes for Network Prefix", heading, fmt.Sprintf("/api/%s/export/%s/prefix-routes?prefix=%s", server.ID, protocol.ID, url.QueryEscape(prefix)), false, false)
}

func (a *App) invalidRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	a.routePage(w, r, "invalid_routes", "Invalid Routes", fmt.Sprintf("Routes with invalid BGP communities on server: %s", server.Name), fmt.Sprintf("/api/%s/invalid-routes", server.ID), true, true)
}

func (a *App) filteredRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	a.routePage(w, r, "filtered_routes", "Filtered Routes", fmt.Sprintf("Routes with selected BGP communities on server: %s", server.Name), fmt.Sprintf("/api/%s/filtered-routes", server.ID), true, false)
}

func (a *App) tableInvalidRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	heading := fmt.Sprintf("Routes with invalid BGP communities from table: %s %s on server: %s", table.ID, table.Peer.DisplayName(), server.Name)
	a.routePage(w, r, "table_invalid_routes", "Table Invalid Routes", heading, fmt.Sprintf("/api/%s/table/%s/invalid-routes", server.ID, table.ID), false, true)
}

func (a *App) tableFilteredRoutesPage(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	heading := fmt.Sprintf("Routes with selected BGP communities from table: %s %s on server: %s", table.ID, table.Peer.DisplayName(), server.Name)
	a.routePage(w, r, "table_filtered_routes", "Table Filtered Routes", heading, fmt.Sprintf("/api/%s/table/%s/filtered-routes", server.ID, table.ID), false, false)
}

func (a *App) bgpProtocolsAPI(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	response, err := a.bgpSvc.JSON(server)
	writeJSONOrError(w, response, err)
}

func (a *App) bfdSessionsAPI(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	response, err := a.bfdSvc.JSON(server)
	writeJSONOrError(w, response, err)
}

func (a *App) protocolDetailAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.bgpSvc.Detail(r.URL.Query().Get("protocol_id")))
}

func (a *App) routeDetailAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.routeSvc.Detail(r.URL.Query().Get("table_id"), r.URL.Query().Get("route_id")))
}

func (a *App) tableRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	params := NewCommandParameters()
	params.Table = table.ID
	results, err := a.routeSvc.TableRoutes(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) prefixTableRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		writeAPIError(w, err, http.StatusBadRequest)
		return
	}
	params := NewCommandParameters()
	params.Table = table.ID
	params.Prefix = prefix
	results, err := a.routeSvc.TableRoutesForPrefix(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) communityTableRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	communities, err := communitySelectorsFromQuery(r.URL.Query().Get("c"), r.URL.Query().Get("lgc"), r.URL.Query().Get("ec"))
	if err != nil {
		writeAPIError(w, err, http.StatusBadRequest)
		return
	}
	params := NewCommandParameters()
	params.Table = table.ID
	params.BgpCommunitiesCondition = "&&"
	params.BgpCommunities = communities
	results, err := a.routeSvc.TableRoutesByCommunity(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) importedRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	params := NewCommandParameters()
	params.Protocol = protocol.ID
	results, err := a.routeSvc.ImportedRoutes(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) prefixImportedRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		writeAPIError(w, err, http.StatusBadRequest)
		return
	}
	params := NewCommandParameters()
	params.Protocol = protocol.ID
	params.Prefix = prefix
	results, err := a.routeSvc.ImportedRoutesForPrefix(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) exportedRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	params := NewCommandParameters()
	params.Export = protocol.ID
	results, err := a.routeSvc.ExportedRoutes(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) prefixExportedRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, protocol, err := a.serverAndProtocol(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	prefix, err := normalizePrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		writeAPIError(w, err, http.StatusBadRequest)
		return
	}
	params := NewCommandParameters()
	params.Export = protocol.ID
	params.Prefix = prefix
	results, err := a.routeSvc.ExportedRoutesForPrefix(server, params)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) invalidRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	results, err := a.routeSvc.InvalidRoutes(server)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) filteredRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, err := a.currentServer(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	results, err := a.routeSvc.FilteredRoutes(server)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) tableInvalidRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	results, err := a.routeSvc.TableInvalidRoutes(server, table.ID)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) tableFilteredRoutesAPI(w http.ResponseWriter, r *http.Request) {
	server, table, err := a.serverAndTable(r)
	if err != nil {
		writeAPIError(w, err, http.StatusNotFound)
		return
	}
	results, err := a.routeSvc.TableFilteredRoutes(server, table.ID)
	writeJSONOrError(w, a.routeSvc.JSON(results), err)
}

func (a *App) currentServer(r *http.Request) (RouteServer, error) {
	serverID := r.PathValue("server")
	if serverID == "" {
		serverID = a.cfg.DefaultServer
	}
	return a.serverSvc.Server(serverID)
}

func (a *App) serverAndTable(r *http.Request) (RouteServer, Symbol, error) {
	server, err := a.currentServer(r)
	if err != nil {
		return RouteServer{}, Symbol{}, err
	}
	table, err := a.symbolSvc.Table(server, r.PathValue("table"))
	return server, table, err
}

func (a *App) serverAndProtocol(r *http.Request) (RouteServer, Symbol, error) {
	server, err := a.currentServer(r)
	if err != nil {
		return RouteServer{}, Symbol{}, err
	}
	protocol, err := a.symbolSvc.Protocol(server, r.PathValue("protocol"))
	return server, protocol, err
}

func serverSubtitle(a *App, r *http.Request) string {
	server, err := a.currentServer(r)
	if err != nil {
		return ""
	}
	return server.Name
}

func (a *App) sortedTables(server RouteServer) []Symbol {
	tables, err := a.symbolSvc.Tables(server)
	if err != nil {
		return nil
	}
	return sortedSymbols(tables)
}

func (a *App) sortedProtocols(server RouteServer) []Symbol {
	protocols, err := a.symbolSvc.Protocols(server)
	if err != nil {
		return nil
	}
	return sortedSymbols(protocols)
}

func sortedSymbols(symbols map[string]Symbol) []Symbol {
	ids := make([]string, 0, len(symbols))
	for id := range symbols {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	results := make([]Symbol, 0, len(ids))
	for _, id := range ids {
		results = append(results, symbols[id])
	}
	return results
}

func (a *App) networkSources(server RouteServer) []SelectOption {
	groups := []struct {
		name    string
		kind    string
		symbols []Symbol
	}{
		{name: "Table", kind: "table", symbols: a.sortedTables(server)},
		{name: "Imported from Protocol", kind: "imported_protocol", symbols: a.sortedProtocols(server)},
		{name: "Exported to Protocol", kind: "exported_protocol", symbols: a.sortedProtocols(server)},
	}
	results := []SelectOption{}
	for _, group := range groups {
		for i, symbol := range group.symbols {
			results = append(results, SelectOption{
				Value:      sourceValue(group.kind, symbol.ID),
				Label:      symbol.ID,
				Subtext:    symbol.Peer.DisplayName(),
				Group:      group.name,
				GroupStart: i == 0,
				GroupEnd:   i == len(group.symbols)-1,
			})
		}
	}
	return results
}

func (a *App) communityOptions() []SelectOption {
	return append(append(
		communityOptionsFromMap("Community", "c", a.cfg.Known.Communities),
		communityOptionsFromMap("Large Community", "lgc", a.cfg.Known.LargeCommunities)...),
		communityOptionsFromMap("Extended Community", "ec", a.cfg.Known.ExtendedCommunities)...)
}

func (a *App) routeCommunityOptions() []SelectOption {
	results := []SelectOption{}
	for _, option := range a.communityOptions() {
		option.Value = strings.Replace(option.Value, ":", "_(", 1) + ")"
		results = append(results, option)
	}
	return results
}

func communityOptionsFromMap(group, prefix string, values map[string]CommunityInfo) []SelectOption {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := []SelectOption{}
	for i, key := range keys {
		info := values[key]
		content := fmt.Sprintf("%s <span class='badge badge-%s'>%s</span>", rawCommunity(key, ""), htmlAttr(info.Label), htmlAttr(info.Name))
		results = append(results, SelectOption{
			Value:      prefix + ":" + key,
			Label:      info.Name,
			Group:      group,
			Content:    template.HTML(content),
			GroupStart: i == 0,
			GroupEnd:   i == len(keys)-1,
		})
	}
	return results
}

func sortedFlags(flags map[string]Flag) []Flag {
	results := make([]Flag, 0, len(flags))
	for _, flag := range flags {
		results = append(results, flag)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Weight == results[j].Weight {
			return results[i].ID < results[j].ID
		}
		return results[i].Weight < results[j].Weight
	})
	return results
}

func writeJSONOrError(w http.ResponseWriter, response jsonResponse, err error) {
	if err != nil {
		writeAPIError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, response)
}

func writeJSON(w http.ResponseWriter, response jsonResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func writeAPIError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": err.Error(),
		"data":  []any{},
	})
}
