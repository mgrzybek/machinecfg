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
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Serial string `json:"serial"`

	Cluster struct {
		Name string `json:"name"`
	} `json:"cluster"`

	Role struct {
		Name string `json:"name"`
	} `json:"DeviceRole"`

	Site struct {
		Slug string `json:"slug"`
	} `json:"site"`

	Tenant struct {
		Slug string `json:"slug"`
	} `json:"tenant"`

	ConfigContext struct {
		Interfaces struct {
			All []struct {
				IP          string `json:"ip"`
				Mtu         int    `json:"mtu"`
				Name        string `json:"name"`
				Prefix      int    `json:"prefix"`
				Shutdown    bool   `json:"shutdown"`
				Description string `json:"description"`
			} `json:"all"`
		} `json:"interfaces"`
	} `json:"config_context"`
}
