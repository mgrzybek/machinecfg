package domain

type ConfigurationArgs struct {
	Endpoint        string
	OutputDirectory string
	Role            string
	Site            string
	Tenant          string
	Token           string
}

type ButaneArgs struct {
	Profile  string
	Template string
}
