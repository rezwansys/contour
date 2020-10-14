// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v2

import (
	"testing"

	"github.com/projectcontour/contour/internal/featuretests"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDownstreamTLSCertificateValidation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	serverTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "serverTLSSecret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}
	rh.OnAdd(serverTLSSecret)

	clientCASecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clientCASecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			dag.CACertificateKey: []byte(featuretests.CERTIFICATE),
		},
	}
	rh.OnAdd(clientCASecret)

	service := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(service)

	proxy := fixture.NewProxy("example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: clientCASecret.Name,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})

	rh.OnAdd(proxy)

	ingress_http := &envoy_api_v2.Listener{
		Name:    "ingress_http",
		Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy_v2.FilterChains(
			envoy_v2.HTTPConnectionManager("ingress_http", envoy_v2.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
		SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
	}

	ingress_https := &envoy_api_v2.Listener{
		Name:    "ingress_https",
		Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v2.ListenerFilters(
			envoy_v2.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificate: &dag.Secret{
						Object: clientCASecret,
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy).Like(
		contour_api_v1.HTTPProxyStatus{CurrentStatus: string(status.ProxyStatusValid)},
	)

}
