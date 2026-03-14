package protomcp

// ResourceContent represents a single content item returned by a resource handler.
type ResourceContent struct {
	URI      string
	MimeType string
	Text     string
	Blob     []byte
}

// ResourceDef represents a registered static resource.
type ResourceDef struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Size        int64
	HandlerFn   func() []ResourceContent
}

// ResourceTemplateDef represents a registered resource template.
type ResourceTemplateDef struct {
	URITemplate string
	Name        string
	Description string
	MimeType    string
	HandlerFn   func(uri string) []ResourceContent
}

var resourceRegistry []ResourceDef
var resourceTemplateRegistry []ResourceTemplateDef

// RegisterResource adds a static resource to the global registry.
func RegisterResource(def ResourceDef) {
	resourceRegistry = append(resourceRegistry, def)
}

// RegisterResourceTemplate adds a resource template to the global registry.
func RegisterResourceTemplate(def ResourceTemplateDef) {
	resourceTemplateRegistry = append(resourceTemplateRegistry, def)
}

// GetRegisteredResources returns a copy of all registered resources.
func GetRegisteredResources() []ResourceDef {
	return append([]ResourceDef{}, resourceRegistry...)
}

// GetRegisteredResourceTemplates returns a copy of all registered resource templates.
func GetRegisteredResourceTemplates() []ResourceTemplateDef {
	return append([]ResourceTemplateDef{}, resourceTemplateRegistry...)
}

// ClearResourceRegistry resets the resource registries.
func ClearResourceRegistry() {
	resourceRegistry = nil
	resourceTemplateRegistry = nil
}

// Resource is a convenience function to register a static resource.
func Resource(uri, description string, handler func() []ResourceContent) {
	RegisterResource(ResourceDef{
		URI:         uri,
		Name:        uri,
		Description: description,
		HandlerFn:   handler,
	})
}

// TextResource registers a static resource that returns a single text content.
func TextResource(uri, description, text string) {
	Resource(uri, description, func() []ResourceContent {
		return []ResourceContent{{URI: uri, Text: text}}
	})
}

// ResourceTemplate is a convenience function to register a resource template.
func ResourceTemplate(uriTemplate, description string, handler func(uri string) []ResourceContent) {
	RegisterResourceTemplate(ResourceTemplateDef{
		URITemplate: uriTemplate,
		Name:        uriTemplate,
		Description: description,
		HandlerFn:   handler,
	})
}
