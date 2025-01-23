package internal

// Feature represents a feature that can be enabled or disabled
// along with any metadata required for it to work
type Feature struct {
	Name     string
	Enabled  bool
	Metadata map[string]any
}

type P0ProviderData struct {
	Client   P0ProviderClient
	Features map[string]Feature
}
