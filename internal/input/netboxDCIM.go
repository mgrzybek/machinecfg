package input

import (
	"time"
)

type APIDevicesResponse struct {
	Count   int      `json:"count"`
	Next    string   `json:"next"`
	Results []Device `json:"results"`
}

type APIDeviceInterfaceResponse struct {
	Count   int               `json:"count"`
	Results []DeviceInterface `json:"results"`
}

type DeviceInterface struct {
	ID         int    `json:"id"`
	URL        string `json:"url"`
	DisplayURL string `json:"display_url"`
	Display    string `json:"display"`
	Device     struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"device"`
	Vdcs   []interface{} `json:"vdcs"`
	Module interface{}   `json:"module"`
	Name   string        `json:"name"`
	Label  string        `json:"label"`
	Type   struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"type"`
	Enabled bool        `json:"enabled"`
	Parent  interface{} `json:"parent"`
	Bridge  interface{} `json:"bridge"`
	Lag     struct {
		ID         int    `json:"id"`
		URL        string `json:"url"`
		DisplayURL string `json:"display_url"`
		Display    string `json:"display"`
		Device     struct {
			ID         int    `json:"id"`
			URL        string `json:"url"`
			DisplayURL string `json:"display_url"`
			Display    string `json:"display"`
			Name       string `json:"name"`
		} `json:"device"`
		Name     string      `json:"name"`
		Cable    interface{} `json:"cable"`
		Occupied bool        `json:"_occupied"`
	} `json:"lag"`
	MTU                         int           `json:"mtu"`
	MacAddress                  interface{}   `json:"mac_address"`
	PrimaryMacAddress           interface{}   `json:"primary_mac_address"`
	MacAddresses                []interface{} `json:"mac_addresses"`
	Speed                       interface{}   `json:"speed"`
	Duplex                      interface{}   `json:"duplex"`
	Wwn                         interface{}   `json:"wwn"`
	MgmtOnly                    bool          `json:"mgmt_only"`
	Description                 string        `json:"description"`
	Mode                        interface{}   `json:"mode"`
	RfRole                      interface{}   `json:"rf_role"`
	RfChannel                   interface{}   `json:"rf_channel"`
	PoeMode                     interface{}   `json:"poe_mode"`
	PoeType                     interface{}   `json:"poe_type"`
	RfChannelFrequency          interface{}   `json:"rf_channel_frequency"`
	RfChannelWidth              interface{}   `json:"rf_channel_width"`
	TxPower                     interface{}   `json:"tx_power"`
	UntaggedVlan                interface{}   `json:"untagged_vlan"`
	TaggedVlans                 []interface{} `json:"tagged_vlans"`
	QinqSvlan                   interface{}   `json:"qinq_svlan"`
	VlanTranslationPolicy       interface{}   `json:"vlan_translation_policy"`
	MarkConnected               bool          `json:"mark_connected"`
	Cable                       interface{}   `json:"cable"`
	CableEnd                    interface{}   `json:"cable_end"`
	WirelessLink                interface{}   `json:"wireless_link"`
	LinkPeers                   []interface{} `json:"link_peers"`
	LinkPeersType               interface{}   `json:"link_peers_type"`
	WirelessLans                []interface{} `json:"wireless_lans"`
	Vrf                         interface{}   `json:"vrf"`
	L2VpnTermination            interface{}   `json:"l2vpn_termination"`
	ConnectedEndpoints          interface{}   `json:"connected_endpoints"`
	ConnectedEndpointsType      interface{}   `json:"connected_endpoints_type"`
	ConnectedEndpointsReachable interface{}   `json:"connected_endpoints_reachable"`
	Tags                        []interface{} `json:"tags"`
	CustomFields                struct {
	} `json:"custom_fields"`
	Created          time.Time `json:"created"`
	LastUpdated      time.Time `json:"last_updated"`
	CountIpaddresses int       `json:"count_ipaddresses"`
	CountFhrpGroups  int       `json:"count_fhrp_groups"`
	Occupied         bool      `json:"_occupied"`
}

type Device struct {
	ID         int    `json:"id"`
	URL        string `json:"url"`
	DisplayURL string `json:"display_url"`
	Display    string `json:"display"`
	Name       string `json:"name"`
	DeviceType struct {
		ID           int    `json:"id"`
		URL          string `json:"url"`
		Display      string `json:"display"`
		Manufacturer struct {
			ID          int    `json:"id"`
			URL         string `json:"url"`
			Display     string `json:"display"`
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
		} `json:"manufacturer"`
		Model       string `json:"model"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	} `json:"device_type"`
	Role struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Depth       int    `json:"_depth"`
	} `json:"role"`
	Tenant struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	} `json:"tenant"`
	Platform struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	} `json:"platform"`
	Serial   string `json:"serial"`
	AssetTag any    `json:"asset_tag"`
	Site      Site `json:"site"`
	Location struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		RackCount   int    `json:"rack_count"`
		Depth       int    `json:"_depth"`
	} `json:"location"`
	Rack struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Display     string `json:"display"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"rack"`
	Position float32 `json:"position"`
	Face     struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"face"`
	Latitude     any `json:"latitude"`
	Longitude    any `json:"longitude"`
	ParentDevice any `json:"parent_device"`
	Status       struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"status"`
	Airflow        any    `json:"airflow"`
	PrimaryIP      any    `json:"primary_ip"`
	PrimaryIP4     any    `json:"primary_ip4"`
	PrimaryIP6     any    `json:"primary_ip6"`
	OobIP          any    `json:"oob_ip"`
	Cluster        any    `json:"cluster"`
	VirtualChassis any    `json:"virtual_chassis"`
	VcPosition     any    `json:"vc_position"`
	VcPriority     any    `json:"vc_priority"`
	Description    string `json:"description"`
	Comments       string `json:"comments"`
	ConfigTemplate any    `json:"config_template"`
	ConfigContext  struct {
	} `json:"config_context"`
	LocalContextData any   `json:"local_context_data"`
	Tags             []any `json:"tags"`
	CustomFields     struct {
	} `json:"custom_fields"`
	Created                time.Time `json:"created"`
	LastUpdated            time.Time `json:"last_updated"`
	ConsolePortCount       int       `json:"console_port_count"`
	ConsoleServerPortCount int       `json:"console_server_port_count"`
	PowerPortCount         int       `json:"power_port_count"`
	PowerOutletCount       int       `json:"power_outlet_count"`
	InterfaceCount         int       `json:"interface_count"`
	FrontPortCount         int       `json:"front_port_count"`
	RearPortCount          int       `json:"rear_port_count"`
	DeviceBayCount         int       `json:"device_bay_count"`
	ModuleBayCount         int       `json:"module_bay_count"`
	InventoryItemCount     int       `json:"inventory_item_count"`
}
