package common

type DeviceFilters struct {
	Regions   []string
	Sites     []string
	Locations []string
	Racks     []int32
	Tenants   []string
	Roles     []string

	Virtualisation bool
	Clusters       []string
}
