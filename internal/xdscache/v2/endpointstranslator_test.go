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

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestEndpointsTranslatorContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_api_v2.ClusterLoadAssignment
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: clusterloadassignments(
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			),
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			et.entries = tc.contents
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_api_v2.ClusterLoadAssignment
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: clusterloadassignments(
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/httpbin-org"},
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			},
		},
		"partial match": {
			contents: clusterloadassignments(
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/kuard/8080", "default/httpbin-org"},
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
				envoy_v2.ClusterLoadAssignment("default/kuard/8080"),
			},
		},
		"no match": {
			contents: clusterloadassignments(
				envoy_v2.ClusterLoadAssignment("default/httpbin-org",
					envoy_v2.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/kuard/8080"},
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/kuard/8080"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			et.entries = tc.contents
			got := et.Query(tc.query)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorAddEndpoints(t *testing.T) {
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/httpbin-org/a",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{Name: "a"},
				},
			},
		},
		{
			ClusterName: "default/httpbin-org/b",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{Name: "b"},
				},
			},
		},
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
			},
		},
	}

	tests := map[string]struct {
		ep   *v1.Endpoints
		want []proto.Message
	}{
		"simple": {
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080)),
				},
			},
		},
		"multiple addresses": {
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
					"23.23.247.89",
				),
				Ports: ports(
					port("", 80),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("23.23.247.89", 80), // addresses should be sorted
						envoy_v2.SocketAddress("50.17.192.147", 80),
						envoy_v2.SocketAddress("50.17.206.192", 80),
						envoy_v2.SocketAddress("50.19.99.160", 80),
					),
				},
			},
		},
		"multiple ports": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
					port("a", 8675),
				),
			}),
			want: []proto.Message{
				// Results should be sorted by cluster name.
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints:   envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("10.10.1.1", 8675)),
				},
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints:   envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("10.10.1.1", 309)),
				},
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
		},
		"cartesian product": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.2.2",
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
					port("a", 8675),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("10.10.1.1", 8675), // addresses should be sorted
						envoy_v2.SocketAddress("10.10.2.2", 8675),
					),
				},
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("10.10.1.1", 309),
						envoy_v2.SocketAddress("10.10.2.2", 309),
					),
				},
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
		},
		"not ready": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				NotReadyAddresses: addresses(
					"10.10.2.2",
				),
				Ports: ports(
					port("a", 8675),
				),
			}, v1.EndpointSubset{
				Addresses: addresses(
					"10.10.2.2",
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("10.10.1.1", 8675),
					),
				},
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("10.10.1.1", 309),
						envoy_v2.SocketAddress("10.10.2.2", 309),
					),
				},
				&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			require.NoError(t, et.cache.SetClusters(clusters))
			et.OnAdd(tc.ep)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorRemoveEndpoints(t *testing.T) {
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
			},
		},
		{
			ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "what-a-descriptive-service-name-you-must-be-so-proud",
					ServiceNamespace: "super-long-namespace-name-oh-boy",
					ServicePort:      v1.ServicePort{Name: "http"},
				},
			},
		},
		{
			ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "what-a-descriptive-service-name-you-must-be-so-proud",
					ServiceNamespace: "super-long-namespace-name-oh-boy",
					ServicePort:      v1.ServicePort{Name: "https"},
				},
			},
		},
	}

	tests := map[string]struct {
		setup func(*EndpointsTranslator)
		ep    *v1.Endpoints
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}))
			},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/simple"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
		},
		"remove different": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}))
			},
			ep: endpoints("default", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080)),
				},
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/simple"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
		},
		"remove long name": {
			setup: func(et *EndpointsTranslator) {
				e1 := endpoints(
					"super-long-namespace-name-oh-boy",
					"what-a-descriptive-service-name-you-must-be-so-proud",
					v1.EndpointSubset{
						Addresses: addresses(
							"172.16.0.2",
							"172.16.0.1",
						),
						Ports: ports(
							port("https", 8443),
							port("http", 8080),
						),
					},
				)
				et.OnAdd(e1)
			},
			ep: endpoints(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				v1.EndpointSubset{
					Addresses: addresses(
						"172.16.0.2",
						"172.16.0.1",
					),
					Ports: ports(
						port("https", 8443),
						port("http", 8080),
					),
				},
			),
			want: []proto.Message{
				envoy_v2.ClusterLoadAssignment("default/simple"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v2.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			require.NoError(t, et.cache.SetClusters(clusters))
			tc.setup(et)
			// TODO(jpeach): this doesn't actually test
			// that deleting endpoints works. We ought to
			// ensure the cache is populated first and
			// only after that, verify that deletion gives
			// the expected result.
			et.OnDelete(tc.ep)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		cluster dag.ServiceCluster
		ep      *v1.Endpoints
		want    []proto.Message
	}{
		"simple": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/simple",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
				}},
			},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("192.168.183.24", 8080)),
				},
			},
		},
		"multiple addresses": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/httpbin-org",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
				}},
			},
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"23.23.247.89",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(
					port("", 80),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("23.23.247.89", 80),
						envoy_v2.SocketAddress("50.17.192.147", 80),
						envoy_v2.SocketAddress("50.17.206.192", 80),
						envoy_v2.SocketAddress("50.19.99.160", 80),
					),
				},
			},
		},
		"named container port": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/secure/https",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "secure",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{Name: "https"},
				}},
			},
			ep: endpoints("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("https", 8443),
				),
			}),
			want: []proto.Message{
				&envoy_api_v2.ClusterLoadAssignment{
					ClusterName: "default/secure/https",
					Endpoints: envoy_v2.WeightedEndpoints(1,
						envoy_v2.SocketAddress("192.168.183.24", 8443)),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			require.NoError(t, et.cache.SetClusters([]*dag.ServiceCluster{&tc.cluster}))
			et.OnAdd(tc.ep)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

// See #602
func TestEndpointsTranslatorScaleToZeroEndpoints(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))

	require.NoError(t, et.cache.SetClusters([]*dag.ServiceCluster{
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{{
				Weight:           1,
				ServiceName:      "simple",
				ServiceNamespace: "default",
				ServicePort:      v1.ServicePort{},
			}},
		},
	}))

	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: ports(
			port("", 8080),
		),
	})
	et.OnAdd(e1)

	// Assert endpoint was added
	want := []proto.Message{
		&envoy_api_v2.ClusterLoadAssignment{
			ClusterName: "default/simple",
			Endpoints:   envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080)),
		},
	}

	protobuf.RequireEqual(t, want, et.Contents())

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	et.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = []proto.Message{
		&envoy_api_v2.ClusterLoadAssignment{ClusterName: "default/simple"},
	}

	protobuf.RequireEqual(t, want, et.Contents())
}

// Test that a cluster with weighted services propagates the weights.
func TestEndpointsTranslatorWeightedService(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/weighted",
			Services: []dag.WeightedService{
				{
					Weight:           0,
					ServiceName:      "weight0",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
				{
					Weight:           1,
					ServiceName:      "weight1",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
				{
					Weight:           2,
					ServiceName:      "weight2",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
			},
		},
	}

	require.NoError(t, et.cache.SetClusters(clusters))

	epSubset := v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(port("", 8080)),
	}

	et.OnAdd(endpoints("default", "weight0", epSubset))
	et.OnAdd(endpoints("default", "weight1", epSubset))
	et.OnAdd(endpoints("default", "weight2", epSubset))

	// Each helper builds a `LocalityLbEndpoints` with one
	// entry, so we can compose the final result by reaching
	// in an taking the first element of each slice.
	w0 := envoy_v2.Endpoints(envoy_v2.SocketAddress("192.168.183.24", 8080))
	w1 := envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080))
	w2 := envoy_v2.WeightedEndpoints(2, envoy_v2.SocketAddress("192.168.183.24", 8080))

	want := []proto.Message{
		&envoy_api_v2.ClusterLoadAssignment{
			ClusterName: "default/weighted",
			Endpoints: []*envoy_api_v2_endpoint.LocalityLbEndpoints{
				w0[0], w1[0], w2[0],
			},
		},
	}

	protobuf.ExpectEqual(t, want, et.Contents())
}

// Test that a cluster with weighted services that all leave the
// weights unspecified defaults to equally weighed and propagates the
// weights.
func TestEndpointsTranslatorDefaultWeightedService(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/weighted",
			Services: []dag.WeightedService{
				{
					ServiceName:      "weight0",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
				{
					ServiceName:      "weight1",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
				{
					ServiceName:      "weight2",
					ServiceNamespace: "default",
					ServicePort:      v1.ServicePort{},
				},
			},
		},
	}

	require.NoError(t, et.cache.SetClusters(clusters))

	epSubset := v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(port("", 8080)),
	}

	et.OnAdd(endpoints("default", "weight0", epSubset))
	et.OnAdd(endpoints("default", "weight1", epSubset))
	et.OnAdd(endpoints("default", "weight2", epSubset))

	// Each helper builds a `LocalityLbEndpoints` with one
	// entry, so we can compose the final result by reaching
	// in an taking the first element of each slice.
	w0 := envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080))
	w1 := envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080))
	w2 := envoy_v2.WeightedEndpoints(1, envoy_v2.SocketAddress("192.168.183.24", 8080))

	want := []proto.Message{
		&envoy_api_v2.ClusterLoadAssignment{
			ClusterName: "default/weighted",
			Endpoints: []*envoy_api_v2_endpoint.LocalityLbEndpoints{
				w0[0], w1[0], w2[0],
			},
		},
	}

	protobuf.ExpectEqual(t, want, et.Contents())
}

func ports(eps ...v1.EndpointPort) []v1.EndpointPort {
	return eps
}

func port(name string, port int32) v1.EndpointPort {
	return v1.EndpointPort{
		Name:     name,
		Port:     port,
		Protocol: "TCP",
	}
}

func clusterloadassignments(clas ...*envoy_api_v2.ClusterLoadAssignment) map[string]*envoy_api_v2.ClusterLoadAssignment {
	m := make(map[string]*envoy_api_v2.ClusterLoadAssignment)
	for _, cla := range clas {
		m[cla.ClusterName] = cla
	}
	return m
}
