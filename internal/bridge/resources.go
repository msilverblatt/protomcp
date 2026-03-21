package bridge

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// ResourceBackend is the interface for resource operations.
type ResourceBackend interface {
	ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error)
	ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error)
	ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error)
}

func syncResources(server *mcp.Server, backend ResourceBackend, registeredRes map[string]bool, registeredTmpl map[string]bool) {
	ctx := context.Background()

	// Sync resources.
	resources, err := backend.ListResources(ctx)
	if err != nil {
		return
	}
	currentRes := make(map[string]bool, len(resources))
	for _, r := range resources {
		currentRes[r.Uri] = true
		res := &mcp.Resource{
			URI:         r.Uri,
			Name:        r.Name,
			Description: r.Description,
			MIMEType:    r.MimeType,
		}
		handler := makeResourceHandler(backend, r.Uri)
		server.AddResource(res, handler)
	}
	var staleRes []string
	for uri := range registeredRes {
		if !currentRes[uri] {
			staleRes = append(staleRes, uri)
		}
	}
	if len(staleRes) > 0 {
		server.RemoveResources(staleRes...)
	}
	for uri := range registeredRes {
		delete(registeredRes, uri)
	}
	for uri := range currentRes {
		registeredRes[uri] = true
	}

	// Sync resource templates.
	templates, err := backend.ListResourceTemplates(ctx)
	if err != nil {
		return
	}
	currentTmpl := make(map[string]bool, len(templates))
	for _, t := range templates {
		currentTmpl[t.UriTemplate] = true
		tmpl := &mcp.ResourceTemplate{
			URITemplate: t.UriTemplate,
			Name:        t.Name,
			Description: t.Description,
			MIMEType:    t.MimeType,
		}
		handler := makeResourceHandler(backend, t.UriTemplate)
		server.AddResourceTemplate(tmpl, handler)
	}
	var staleTmpl []string
	for uri := range registeredTmpl {
		if !currentTmpl[uri] {
			staleTmpl = append(staleTmpl, uri)
		}
	}
	if len(staleTmpl) > 0 {
		server.RemoveResourceTemplates(staleTmpl...)
	}
	for uri := range registeredTmpl {
		delete(registeredTmpl, uri)
	}
	for uri := range currentTmpl {
		registeredTmpl[uri] = true
	}
}

func makeResourceHandler(backend ResourceBackend, uri string) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		resp, err := backend.ReadResource(ctx, req.Params.URI)
		if err != nil {
			return nil, err
		}
		result := &mcp.ReadResourceResult{}
		for _, c := range resp.Contents {
			rc := &mcp.ResourceContents{
				URI:      c.Uri,
				MIMEType: c.MimeType,
			}
			if len(c.Blob) > 0 {
				rc.Blob = c.Blob
			} else {
				rc.Text = c.Text
			}
			result.Contents = append(result.Contents, rc)
		}
		return result, nil
	}
}
