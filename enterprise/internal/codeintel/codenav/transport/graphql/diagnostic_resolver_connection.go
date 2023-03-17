package graphql

import (
	"github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel/codenav/shared"
	sharedresolvers "github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel/shared/resolvers"
	resolverstubs "github.com/sourcegraph/sourcegraph/internal/codeintel/resolvers"
)

type diagnosticConnectionResolver struct {
	diagnostics      []shared.DiagnosticAtUpload
	totalCount       int
	locationResolver *sharedresolvers.CachedLocationResolver
}

func NewDiagnosticConnectionResolver(diagnostics []shared.DiagnosticAtUpload, totalCount int, locationResolver *sharedresolvers.CachedLocationResolver) resolverstubs.DiagnosticConnectionResolver {
	return &diagnosticConnectionResolver{
		diagnostics:      diagnostics,
		totalCount:       totalCount,
		locationResolver: locationResolver,
	}
}

func (r *diagnosticConnectionResolver) Nodes() []resolverstubs.DiagnosticResolver {
	resolvers := make([]resolverstubs.DiagnosticResolver, 0, len(r.diagnostics))
	for i := range r.diagnostics {
		resolvers = append(resolvers, NewDiagnosticResolver(r.diagnostics[i], r.locationResolver))
	}
	return resolvers
}

func (r *diagnosticConnectionResolver) TotalCount() *int32 {
	v := int32(r.totalCount)
	return &v
}

func (r *diagnosticConnectionResolver) PageInfo() resolverstubs.PageInfo {
	return HasNextPage(len(r.diagnostics) < r.totalCount)
}
