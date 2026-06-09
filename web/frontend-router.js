const routeMap = {
    bgp_protocol_detail_api: () => '/api/protocol/detail',
    route_detail_api: () => '/api/route/detail',
    community_lookup: ({server}) => `/${server}/community/lookup`,
    imported_routes: ({server, protocol}) => `/${server}/protocol/${protocol}/routes`,
    exported_routes: ({server, protocol}) => `/${server}/export/${protocol}/routes`,
    table_routes: ({server, table}) => `/${server}/table/${table}/routes`,
    table_invalid_routes: ({server, table}) => `/${server}/table/${table}/invalid-routes`,
    table_filtered_routes: ({server, table}) => `/${server}/table/${table}/filtered-routes`,
};

const Routing = {
    setRoutingData() {},
    generate(name, parameters = {}) {
        if (!routeMap[name]) {
            throw new Error(`Unknown route: ${name}`);
        }

        return routeMap[name](parameters);
    },
};

export default Routing;
