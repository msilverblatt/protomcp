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

func syncResources(server *mcp.Server, backend ResourceBackend) {
	ctx := context.Background()
	resources, err := backend.ListResources(ctx)
	if err != nil {
		return
	}
	for _, r := range resources {
		res := &mcp.Resource{
			URI:         r.Uri,
			Name:        r.Name,
			Description: r.Description,
			MIMEType:    r.MimeType,
		}
		handler := makeResourceHandler(backend, r.Uri)
		server.AddResource(res, handler)
	}

	templates, err := backend.ListResourceTemplates(ctx)
	if err != nil {
		return
	}
	for _, t := range templates {
		tmpl := &mcp.ResourceTemplate{
			URITemplate: t.UriTemplate,
			Name:        t.Name,
			Description: t.Description,
			MIMEType:    t.MimeType,
		}
		handler := makeResourceHandler(backend, t.UriTemplate)
		server.AddResourceTemplate(tmpl, handler)
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
