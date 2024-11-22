package app

import (
	"dockside/app/util/must"
	"fmt"
	"github.com/maddalax/htmgo/framework/service"
	"github.com/maddalax/multiproxy"
	"net/http"
	"strings"
)

type ConfigBuilder struct {
	matcher        *Matcher
	serviceLocator *service.Locator
}

func CalculateUpstreamId(resourceId string, serverId string, port string) string {
	if strings.HasPrefix(port, ":") {
		port = port[1:]
	}
	return fmt.Sprintf("upstream-res-%s-ser-%s-port-%s", resourceId, serverId, port)
}

func NewConfigBuilder(locator *service.Locator) *ConfigBuilder {
	return &ConfigBuilder{
		matcher:        &Matcher{},
		serviceLocator: locator,
	}
}

func (b *ConfigBuilder) Append(resource *Resource, block *RouteBlock, lb *multiproxy.LoadBalancer[UpstreamMeta]) error {

	if len(resource.ServerDetails) == 0 {
		return nil
	}

	for _, serverDetail := range resource.ServerDetails {
		if serverDetail.RunStatus == RunStatusNotRunning {
			continue
		}
		server, err := ServerGet(b.serviceLocator, serverDetail.ServerId)
		if err != nil {
			continue
		}

		// skip if server is not accessible
		if !server.IsAccessible() {
			continue
		}

		for _, up := range serverDetail.Upstreams {
			upstream := &CustomUpstream{
				Metadata: UpstreamMeta{
					Resource: resource,
					Server:   server,
					Block:    block,
				},
				Id:  CalculateUpstreamId(resource.Id, server.Id, up.Port),
				Url: must.Url(fmt.Sprintf("http://%s:%s", up.Host, up.Port)),
				MatchesFunc: func(u *CustomUpstream, req *http.Request) bool {
					return UpstreamMatches(u, req)
				},
				// really doesn't matter since we are overriding the MatchesFunc
				Matches: []multiproxy.Match{},
			}

			b.matcher.CompileUpstream(upstream)
			lb.AddStagedUpstream(upstream)
		}
	}
	return nil
}