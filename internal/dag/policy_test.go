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

package dag

import (
	"io/ioutil"
	"testing"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryPolicyIngress(t *testing.T) {
	tests := map[string]struct {
		i    *v1beta1.Ingress
		want *RetryPolicy
	}{
		"no anotations": {
			i:    &v1beta1.Ingress{},
			want: nil,
		},
		"retry-on": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on": "5xx",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn: "5xx",
			},
		},
		"explicitly zero retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "0",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 0,
			},
		},
		"legacy explicitly zero retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":     "5xx",
						"contour.heptio.com/num-retries": "0",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 0,
			},
		},
		"num-retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "7",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 7,
			},
		},
		"legacy num-retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":     "5xx",
						"contour.heptio.com/num-retries": "7",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 7,
			},
		},
		"no retry count, per try timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "10s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"no retry count, legacy per try timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":         "5xx",
						"contour.heptio.com/per-try-timeout": "10s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"explicit 0s timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "0s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
		"legacy explicit 0s timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":         "5xx",
						"contour.heptio.com/per-try-timeout": "0s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ingressRetryPolicy(tc.i, &logrus.Logger{Out: ioutil.Discard})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRetryPolicy(t *testing.T) {
	tests := map[string]struct {
		rp   *contour_api_v1.RetryPolicy
		want *RetryPolicy
	}{
		"nil retry policy": {
			rp:   nil,
			want: nil,
		},
		"empty policy": {
			rp: &contour_api_v1.RetryPolicy{},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly zero retries": {
			rp: &contour_api_v1.RetryPolicy{
				NumRetries: 0, // zero value for NumRetries
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"no retry count, per try timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "10s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"explicit 0s timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "0s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
		"retry on": {
			rp: &contour_api_v1.RetryPolicy{
				RetryOn: []contour_api_v1.RetryOn{"gateway-error", "connect-failure"},
			},
			want: &RetryPolicy{
				RetryOn:    "gateway-error,connect-failure",
				NumRetries: 1,
			},
		},
		"retriable status codes": {
			rp: &contour_api_v1.RetryPolicy{
				RetriableStatusCodes: []uint32{502, 503, 504},
			},
			want: &RetryPolicy{
				RetryOn:              "5xx",
				RetriableStatusCodes: []uint32{502, 503, 504},
				NumRetries:           1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retryPolicy(tc.rp)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTimeoutPolicy(t *testing.T) {
	tests := map[string]struct {
		tp      *contour_api_v1.TimeoutPolicy
		want    TimeoutPolicy
		wantErr bool
	}{
		"nil timeout policy": {
			tp:   nil,
			want: TimeoutPolicy{},
		},
		"empty timeout policy": {
			tp:   &contour_api_v1.TimeoutPolicy{},
			want: TimeoutPolicy{},
		},
		"valid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "1m30s",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DurationSetting(90 * time.Second),
			},
		},
		"invalid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "90", // 90 what?
			},
			wantErr: true,
		},
		"infinite response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "infinite",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DisabledSetting(),
			},
		},
		"idle timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Idle: "900s",
			},
			want: TimeoutPolicy{
				IdleTimeout: timeout.DurationSetting(900 * time.Second),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := timeoutPolicy(tc.tp)
			if tc.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, tc.want, got)
				assert.NoError(t, gotErr)
			}

		})
	}
}

func TestLoadBalancerPolicy(t *testing.T) {
	tests := map[string]struct {
		lbp  *contour_api_v1.LoadBalancerPolicy
		want string
	}{
		"nil": {
			lbp:  nil,
			want: "",
		},
		"empty": {
			lbp:  &contour_api_v1.LoadBalancerPolicy{},
			want: "",
		},
		"WeightedLeastRequest": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "WeightedLeastRequest",
			},
			want: "WeightedLeastRequest",
		},
		"Random": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Random",
			},
			want: "Random",
		},
		"Cookie": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			want: "Cookie",
		},
		"unknown": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "please",
			},
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := loadBalancerPolicy(tc.lbp)
			assert.Equal(t, tc.want, got)
		})
	}
}
