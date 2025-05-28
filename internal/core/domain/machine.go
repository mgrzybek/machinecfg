package domain

// MachineInfo represents the tags used to create a machine configuration (generic)
type MachineInfo struct {
	Hostname         string
	Serial           string
	Bondings         []BondingConfiguration
	Interfaces       []PhysicalInterface
	BootstrapURL     string
	JournaldURL      string
	LoggingEndpoints []string
	DNS              []string
	NTPServers       []string
}

// PhysicalInterface represents a network adapter and its configuration
type PhysicalInterface struct {
	MACAddress  string
	Name        string
	MTU         int
	IPsWithCIDR []string
	Gateways    []string
	VLANs       []VLANInterface
	LAG         string
}

// BondingConfiguration represents the required elements needed to configure a network link aggregate
type BondingConfiguration struct {
	Name        string
	Interfaces  []*PhysicalInterface
	IPsWithCIDR []string
	Gateways    []string
	VLANs       []VLANInterface
}

// VLANInterface represents a VLAN ID and IP addresses attached to a network adapter or bonding
type VLANInterface struct {
	ID          int
	IPsWithCIDR []string
}
