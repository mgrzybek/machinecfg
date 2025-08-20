package input

type Status struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type Site struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type Role struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Depth       int    `json:"_depth"`
}
type Family struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type CustomFields struct {
}

type ConfigContext struct {
}

type Type struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Display     string `json:"display"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}
