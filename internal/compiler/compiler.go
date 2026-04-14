/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package compiler transforms chorister CRD specs into Kubernetes manifests.
package compiler

import (
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var (
	httpRouteGVK           = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}
	referenceGrantGVK      = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1beta1", Kind: "ReferenceGrant"}
	ciliumNetworkPolicyGVK = schema.GroupVersionKind{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"}
	ciliumEnvoyConfigGVK   = schema.GroupVersionKind{Group: "cilium.io", Version: "v2", Kind: "CiliumEnvoyConfig"}
)

type LinkArtifacts struct {
	HTTPRoute         *unstructured.Unstructured
	ReferenceGrant    *unstructured.Unstructured
	CiliumPolicy      *unstructured.Unstructured
	CiliumEnvoyConfig *unstructured.Unstructured
	DirectDenyPolicy  *networkingv1.NetworkPolicy
}

func CompileIngressHTTPRoute(network *choristerv1alpha1.ChoNetwork) *unstructured.Unstructured {
	route := newUnstructured(httpRouteGVK, network.Namespace, network.Name+"-ingress")
	annotations := map[string]string{}
	if network.Spec.Ingress != nil && network.Spec.Ingress.Auth != nil && network.Spec.Ingress.Auth.JWT != nil {
		annotations["chorister.dev/jwt-issuer"] = network.Spec.Ingress.Auth.JWT.Issuer
		annotations["chorister.dev/jwt-jwks-uri"] = network.Spec.Ingress.Auth.JWT.JWKSUri
		annotations["chorister.dev/jwt-audiences"] = strings.Join(network.Spec.Ingress.Auth.JWT.Audience, ",")
	}
	route.SetAnnotations(annotations)
	route.Object["spec"] = map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "chorister-internet-gateway"},
		},
		"hostnames": []interface{}{fmt.Sprintf("%s-%s.chorister.internal", network.Spec.Application, network.Spec.Domain)},
		"rules": []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": "/",
						},
					},
				},
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": network.Name,
						"port": network.Spec.Ingress.Port,
					},
				},
			},
		},
	}
	return route
}

func CompileEgressCiliumPolicy(network *choristerv1alpha1.ChoNetwork) *unstructured.Unstructured {
	policy := newUnstructured(ciliumNetworkPolicyGVK, network.Namespace, network.Name+"-egress")
	policy.Object["spec"] = map[string]interface{}{
		"endpointSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"chorister.dev/application": network.Spec.Application,
				"chorister.dev/domain":      network.Spec.Domain,
			},
		},
		"egress": compileEgressRules(network.Spec.Egress),
	}
	return policy
}

func CompileCrossApplicationLink(app *choristerv1alpha1.ChoApplication, link choristerv1alpha1.LinkSpec, consumerDomain string) LinkArtifacts {
	consumerNamespace := fmt.Sprintf("%s-%s", app.Name, consumerDomain)
	targetNamespace := fmt.Sprintf("%s-%s", link.Target, link.TargetDomain)
	baseName := fmt.Sprintf("link-%s-%s", link.Name, consumerDomain)

	httpRoute := newUnstructured(httpRouteGVK, consumerNamespace, baseName)
	httpRoute.Object["spec"] = map[string]interface{}{
		"parentRefs": []interface{}{map[string]interface{}{"name": "chorister-internal-gateway"}},
		"rules": []interface{}{
			map[string]interface{}{
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name":      link.TargetDomain,
						"namespace": targetNamespace,
						"port":      link.Port,
					},
				},
			},
		},
	}

	referenceGrant := newUnstructured(referenceGrantGVK, targetNamespace, baseName)
	referenceGrant.Object["spec"] = map[string]interface{}{
		"from": []interface{}{
			map[string]interface{}{
				"group":     "gateway.networking.k8s.io",
				"kind":      "HTTPRoute",
				"namespace": consumerNamespace,
			},
		},
		"to": []interface{}{
			map[string]interface{}{
				"group": "",
				"kind":  "Service",
				"name":  link.TargetDomain,
			},
		},
	}

	ciliumPolicy := newUnstructured(ciliumNetworkPolicyGVK, consumerNamespace, baseName)
	ciliumPolicy.Object["spec"] = map[string]interface{}{
		"endpointSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"chorister.dev/application": app.Name,
				"chorister.dev/domain":      consumerDomain,
			},
		},
		"egress": []interface{}{
			map[string]interface{}{
				"toEndpoints": []interface{}{
					map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"k8s:io.kubernetes.pod.namespace": targetNamespace,
						},
					},
				},
				"toPorts": []interface{}{
					map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{"port": fmt.Sprintf("%d", link.Port), "protocol": "TCP"},
						},
					},
				},
			},
		},
	}

	envoyConfig := newUnstructured(ciliumEnvoyConfigGVK, consumerNamespace, baseName)
	envoyConfig.Object["spec"] = map[string]interface{}{
		"services": []interface{}{
			map[string]interface{}{
				"name":      link.TargetDomain,
				"namespace": targetNamespace,
			},
		},
		"backendServices": []interface{}{
			map[string]interface{}{
				"name":      link.TargetDomain,
				"namespace": targetNamespace,
			},
		},
		"rateLimit": map[string]interface{}{},
	}
	if link.RateLimit != nil {
		envoyConfig.Object["spec"].(map[string]interface{})["rateLimit"] = map[string]interface{}{
			"requestsPerMinute": link.RateLimit.RequestsPerMinute,
		}
	}
	if link.Auth != nil {
		annotations := envoyConfig.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["chorister.dev/link-auth-type"] = link.Auth.Type
		envoyConfig.SetAnnotations(annotations)
	}

	tcp := intstr.FromInt(link.Port)
	directDenyPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName + "-deny-direct",
			Namespace: consumerNamespace,
			Labels: map[string]string{
				"chorister.dev/link-name": link.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{{Port: &tcp}},
				},
			},
		},
	}

	return LinkArtifacts{
		HTTPRoute:         httpRoute,
		ReferenceGrant:    referenceGrant,
		CiliumPolicy:      ciliumPolicy,
		CiliumEnvoyConfig: envoyConfig,
		DirectDenyPolicy:  directDenyPolicy,
	}
}

func newUnstructured(gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func compileEgressRules(spec *choristerv1alpha1.NetworkEgressSpec) []interface{} {
	rules := []interface{}{
		map[string]interface{}{
			"toPorts": []interface{}{
				map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{"port": "53", "protocol": "UDP"},
						map[string]interface{}{"port": "53", "protocol": "TCP"},
					},
				},
			},
		},
	}
	if spec == nil {
		return rules
	}
	for _, host := range spec.Allowlist {
		rules = append(rules, map[string]interface{}{
			"toFQDNs": []interface{}{map[string]interface{}{"matchName": host}},
			"toPorts": []interface{}{
				map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{"port": "443", "protocol": "TCP"},
					},
				},
			},
		})
	}
	return rules
}
