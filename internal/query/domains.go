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

package query

import (
	"context"
	"fmt"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// DomainInfo aggregates domain metadata from the application spec and status.
type DomainInfo struct {
	Name          string
	Application   string
	Namespace     string
	Sensitivity   string
	Phase         string
	ResourceCount int
	Isolated      bool
}

// ListDomainsByApp returns domain info for all domains in the given application.
func (q *Querier) ListDomainsByApp(ctx context.Context, appName string) ([]DomainInfo, error) {
	app, err := q.GetApplication(ctx, appName)
	if err != nil {
		return nil, err
	}
	return domainsFromApp(ctx, q, app)
}

// ListAllDomains returns domain info across all applications (or filtered to one app).
func (q *Querier) ListAllDomains(ctx context.Context, appFilter string) ([]DomainInfo, error) {
	if appFilter != "" {
		return q.ListDomainsByApp(ctx, appFilter)
	}

	apps, err := q.ListApplications(ctx)
	if err != nil {
		return nil, err
	}

	var all []DomainInfo
	for i := range apps {
		domains, err := domainsFromApp(ctx, q, &apps[i])
		if err != nil {
			return nil, err
		}
		all = append(all, domains...)
	}
	return all, nil
}

func domainsFromApp(_ context.Context, _ *Querier, app *choristerv1alpha1.ChoApplication) ([]DomainInfo, error) {
	domains := make([]DomainInfo, 0, len(app.Spec.Domains))
	for _, d := range app.Spec.Domains {
		ns := ""
		if app.Status.DomainNamespaces != nil {
			ns = app.Status.DomainNamespaces[d.Name]
		}

		sensitivity := d.Sensitivity
		if sensitivity == "" {
			sensitivity = "internal"
		}

		// Check isolation annotation
		isolated := false
		if app.Annotations != nil {
			key := fmt.Sprintf("chorister.dev/isolate-%s", d.Name)
			if app.Annotations[key] == "true" {
				isolated = true
			}
		}

		phase := "Pending"
		if ns != "" {
			phase = "Active"
		}

		domains = append(domains, DomainInfo{
			Name:        d.Name,
			Application: app.Name,
			Namespace:   ns,
			Sensitivity: sensitivity,
			Phase:       phase,
			Isolated:    isolated,
		})
	}
	return domains, nil
}
