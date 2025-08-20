package input

import (
	"time"
)

type APIVirtualMachineResponse struct {
	Count   int      `json:"count"`
	Next    string   `json:"next"`
	Previous    string   `json:"previous"`
	Results []VirtualMachine `json:"results"`
}

type PrimaryIP struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Family      Family `json:"family"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

type PrimaryIP4 struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Family      Family `json:"family"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

type VirtualMachine struct {
	ID               int           `json:"id"`
	URL              string        `json:"url"`
	DisplayURL       string        `json:"display_url"`
	Display          string        `json:"display"`
	Name             string        `json:"name"`
	Status           Status        `json:"status"`
	Site             Site          `json:"site"`
	Cluster          Cluster       `json:"cluster"`
	Device           any           `json:"device"`
	Serial           string        `json:"serial"`
	Role             Role          `json:"role"`
	Tenant           any           `json:"tenant"`
	Platform         any           `json:"platform"`
	PrimaryIP        PrimaryIP     `json:"primary_ip"`
	PrimaryIP4       PrimaryIP4    `json:"primary_ip4"`
	PrimaryIP6       any           `json:"primary_ip6"`
	Vcpus            float32           `json:"vcpus"`
	Memory           int           `json:"memory"`
	Disk             int           `json:"disk"`
	Description      string        `json:"description"`
	Comments         string        `json:"comments"`
	ConfigTemplate   any           `json:"config_template"`
	LocalContextData any           `json:"local_context_data"`
	Tags             []any         `json:"tags"`
	CustomFields     CustomFields  `json:"custom_fields"`
	ConfigContext    ConfigContext `json:"config_context"`
	Created          time.Time     `json:"created"`
	LastUpdated      time.Time     `json:"last_updated"`
	InterfaceCount   int           `json:"interface_count"`
	VirtualDiskCount int           `json:"virtual_disk_count"`
}

type Cluster struct {
	ID                  int          `json:"id"`
	URL                 string       `json:"url"`
	DisplayURL          string       `json:"display_url"`
	Display             string       `json:"display"`
	Name                string       `json:"name"`
	Type                Type         `json:"type"`
	Group               any          `json:"group"`
	Status              Status       `json:"status"`
	Tenant              any          `json:"tenant"`
	ScopeType           any          `json:"scope_type"`
	ScopeID             any          `json:"scope_id"`
	Scope               any          `json:"scope"`
	Description         string       `json:"description"`
	Comments            string       `json:"comments"`
	Tags                []any        `json:"tags"`
	CustomFields        CustomFields `json:"custom_fields"`
	Created             time.Time    `json:"created"`
	LastUpdated         time.Time    `json:"last_updated"`
	DeviceCount         int          `json:"device_count"`
	VirtualmachineCount int          `json:"virtualmachine_count"`
	AllocatedVcpus      int          `json:"allocated_vcpus"`
	AllocatedMemory     int          `json:"allocated_memory"`
	AllocatedDisk       int          `json:"allocated_disk"`
}

type APIVirtualInterfaceResponse struct {
	Count   int      `json:"count"`
	Next    string   `json:"next"`
	Previous    string   `json:"previous"`
	Results []VirtualInterface `json:"results"`
}

type PrimaryMacAddress struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	MacAddress  string `json:"mac_address"`
	Description string `json:"description"`
}

type MacAddresses struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	MacAddress  string `json:"mac_address"`
	Description string `json:"description"`
}

type Mode struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type UntaggedVlan struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Vid         int    `json:"vid"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type VirtualInterface struct {
	ID                    int               `json:"id"`
	URL                   string            `json:"url"`
	DisplayURL            string            `json:"display_url"`
	Display               string            `json:"display"`
	VirtualMachine        VirtualMachine    `json:"virtual_machine"`
	Name                  string            `json:"name"`
	Enabled               bool              `json:"enabled"`
	Parent                any               `json:"parent"`
	Bridge                any               `json:"bridge"`
	Mtu                   int               `json:"mtu"`
	MacAddress            string            `json:"mac_address"`
	PrimaryMacAddress     PrimaryMacAddress `json:"primary_mac_address"`
	MacAddresses          []MacAddresses    `json:"mac_addresses"`
	Description           string            `json:"description"`
	Mode                  Mode              `json:"mode"`
	UntaggedVlan          UntaggedVlan      `json:"untagged_vlan"`
	TaggedVlans           []any             `json:"tagged_vlans"`
	QinqSvlan             any               `json:"qinq_svlan"`
	VlanTranslationPolicy any               `json:"vlan_translation_policy"`
	Vrf                   any               `json:"vrf"`
	L2VpnTermination      any               `json:"l2vpn_termination"`
	Tags                  []any             `json:"tags"`
	CustomFields          CustomFields      `json:"custom_fields"`
	Created               time.Time         `json:"created"`
	LastUpdated           time.Time         `json:"last_updated"`
	CountIpaddresses      int               `json:"count_ipaddresses"`
	CountFhrpGroups       int               `json:"count_fhrp_groups"`
}
