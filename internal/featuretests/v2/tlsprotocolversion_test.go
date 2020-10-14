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
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTLSMinimumProtocolVersion(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}
	rh.OnAdd(sec1)

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *featuretests.Backend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	c.Request(listenerType, "ingress_https").Equals(&envoy_api_v2.DiscoveryResponse{
		Resources: resources(t,
			&envoy_api_v2.Listener{
				Name:    "ingress_https",
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("kuard.example.com", sec1,
						httpsFilterFor("kuard.example.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			},
		),
		TypeUrl: listenerType,
	})

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: sec1.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/tls-minimum-protocol-version": "1.3",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *featuretests.Backend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)

	l1 := &envoy_api_v2.Listener{
		Name:    "ingress_https",
		Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v2.ListenerFilters(
			envoy_v2.TLSInspector(),
		),
		FilterChains: []*envoy_api_v2_listener.FilterChain{
			envoy_v2.FilterChainTLS(
				"kuard.example.com",
				envoy_v2.DownstreamTLSContext(
					&dag.Secret{Object: sec1},
					envoy_api_v2_auth.TlsParameters_TLSv1_3,
					nil,
					"h2", "http/1.1"),
				envoy_v2.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType, "ingress_https").Equals(&envoy_api_v2.DiscoveryResponse{
		Resources: resources(t,
			l1,
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(i2)

	hp1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:             sec1.Namespace + "/" + sec1.Name,
					MinimumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType, "ingress_https").Equals(&envoy_api_v2.DiscoveryResponse{
		Resources: resources(t,
			l1,
		),
		TypeUrl: listenerType,
	})
}
