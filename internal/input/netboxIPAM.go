package input

type APIAddressesResponse struct {
	Count   int         `json:"count"`
	Results []IPAddress `json:"results"`
}

type IPAddressFamily struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type IPAddress struct {
	ID      int             `json:"id"`
	URL     string          `json:"url"`
	Display string          `json:"display"`
	Family  IPAddressFamily `json:"family"`
	Address string          `json:"address"`
	Vrf     any             `json:"vrf"`
	Tenant  struct {
		ID      int    `json:"id"`
		URL     string `json:"url"`
		Display string `json:"display"`
		Name    string `json:"name"`
		Slug    string `json:"slug"`
	} `json:"tenant"`
	Status struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"status"`
	Role               any    `json:"role"`
	AssignedObjectType string `json:"assigned_object_type"`
	AssignedObjectID   int    `json:"assigned_object_id"`
	AssignedObject     struct {
		ID      int    `json:"id"`
		URL     string `json:"url"`
		Display string `json:"display"`
		Device  struct {
			ID      int    `json:"id"`
			URL     string `json:"url"`
			Display string `json:"display"`
			Name    string `json:"name"`
		} `json:"device"`
		Name     string `json:"name"`
		Cable    int    `json:"cable"`
		Occupied bool   `json:"_occupied"`
	} `json:"assigned_object"`
	NatInside    any    `json:"nat_inside"`
	NatOutside   []any  `json:"nat_outside"`
	DNSName      string `json:"dns_name"`
	Description  string `json:"description"`
	Comments     string `json:"comments"`
	Tags         []any  `json:"tags"`
	CustomFields struct {
	} `json:"custom_fields"`
}
