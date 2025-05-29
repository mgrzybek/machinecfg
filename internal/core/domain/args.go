package domain

type ConfigurationArgs struct {
	Endpoint string
	Token    string

	OutputDirectory string

	Region   string
	Site     string
	Location string
	Rack     string

	Tenant string
	Role   string
}

type ButaneArgs struct {
	Profile  string
	Template string
}
