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

package loader

import (
	"testing"
)

func TestLoadFile_SingleCompute(t *testing.T) {
	input := []byte(`
apiVersion: chorister.dev/v1alpha1
kind: ChoCompute
metadata:
  name: api
spec:
  application: myapp
  domain: payments
  image: myregistry/api:v1
`)
	res, err := LoadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Computes) != 1 {
		t.Fatalf("expected 1 compute, got %d", len(res.Computes))
	}
	if res.Computes[0].Name != "api" {
		t.Fatalf("expected name api, got %s", res.Computes[0].Name)
	}
	if res.Computes[0].Spec.Image != "myregistry/api:v1" {
		t.Fatalf("expected image myregistry/api:v1, got %s", res.Computes[0].Spec.Image)
	}
}

func TestLoadFile_MultiDocument(t *testing.T) {
	input := []byte(`---
apiVersion: chorister.dev/v1alpha1
kind: ChoCompute
metadata:
  name: api
spec:
  application: myapp
  domain: payments
  image: myregistry/api:v1
---
apiVersion: chorister.dev/v1alpha1
kind: ChoDatabase
metadata:
  name: main
spec:
  application: myapp
  domain: payments
  engine: postgres
---
apiVersion: chorister.dev/v1alpha1
kind: ChoCache
metadata:
  name: sessions
spec:
  application: myapp
  domain: payments
  size: small
`)
	res, err := LoadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 3 {
		t.Fatalf("expected 3 total resources, got %d", res.Total())
	}
	if len(res.Computes) != 1 {
		t.Fatalf("expected 1 compute, got %d", len(res.Computes))
	}
	if len(res.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(res.Databases))
	}
	if len(res.Caches) != 1 {
		t.Fatalf("expected 1 cache, got %d", len(res.Caches))
	}
}

func TestLoadFile_AllSixTypes(t *testing.T) {
	input := []byte(`---
apiVersion: chorister.dev/v1alpha1
kind: ChoCompute
metadata:
  name: api
spec:
  application: myapp
  domain: payments
  image: myregistry/api:v1
---
apiVersion: chorister.dev/v1alpha1
kind: ChoDatabase
metadata:
  name: main
spec:
  application: myapp
  domain: payments
  engine: postgres
---
apiVersion: chorister.dev/v1alpha1
kind: ChoNetwork
metadata:
  name: public
spec:
  application: myapp
  domain: payments
---
apiVersion: chorister.dev/v1alpha1
kind: ChoCache
metadata:
  name: sessions
spec:
  application: myapp
  domain: payments
---
apiVersion: chorister.dev/v1alpha1
kind: ChoQueue
metadata:
  name: events
spec:
  application: myapp
  domain: payments
  type: nats
---
apiVersion: chorister.dev/v1alpha1
kind: ChoStorage
metadata:
  name: uploads
spec:
  application: myapp
  domain: payments
  variant: object
`)
	res, err := LoadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 6 {
		t.Fatalf("expected 6 total resources, got %d", res.Total())
	}
}

func TestLoadFile_EmptyInput(t *testing.T) {
	_, err := LoadFile([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestLoadFile_UnsupportedKind(t *testing.T) {
	input := []byte(`
apiVersion: chorister.dev/v1alpha1
kind: ChoApplication
metadata:
  name: myapp
`)
	_, err := LoadFile(input)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestLoadFile_WrongAPIVersion(t *testing.T) {
	input := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
`)
	_, err := LoadFile(input)
	if err == nil {
		t.Fatal("expected error for wrong apiVersion")
	}
}
