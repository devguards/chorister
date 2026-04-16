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

// Package web provides the chorister web UI HTTP server.
package web

import (
	"fmt"
	"net/http"
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
	"github.com/chorister-dev/chorister/internal/web/pages"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const controlPlaneNamespace = "cho-system"

// Server is the chorister web UI HTTP server. It reuses internal/query for
// data retrieval and delegates mutations to the K8s API directly (same as CLI).
type Server struct {
	client      client.Client
	querier     *query.Querier
	mux         *http.ServeMux
	defaultUser string // identity used for web-initiated approvals
}

// NewServer creates a new Server using the provided Kubernetes client.
// defaultUser is recorded as the approver identity when no auth is configured.
func NewServer(c client.Client, defaultUser string) *Server {
	if defaultUser == "" {
		defaultUser = "web-user"
	}
	s := &Server{
		client:      c,
		querier:     query.NewQuerier(c),
		mux:         http.NewServeMux(),
		defaultUser: defaultUser,
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Index
	s.mux.HandleFunc("GET /{$}", s.handleIndex)

	// Applications
	s.mux.HandleFunc("GET /apps", s.handleApps)
	s.mux.HandleFunc("GET /apps/{name}", s.handleAppDetail)

	// Sandboxes
	s.mux.HandleFunc("GET /sandboxes", s.handleSandboxes)

	// Promotions
	s.mux.HandleFunc("GET /promotions", s.handlePromotions)
	s.mux.HandleFunc("POST /promotions/{name}/approve", s.handlePromotionApprove)
	s.mux.HandleFunc("POST /promotions/{name}/reject", s.handlePromotionReject)

	// Members
	s.mux.HandleFunc("GET /members", s.handleMembers)

	// Vulnerability reports
	s.mux.HandleFunc("GET /vulns", s.handleVulns)

	// Live events
	s.mux.HandleFunc("GET /events", s.handleEventsPage)

	// SSE stream (consumed by /events page)
	s.mux.HandleFunc("GET /api/events", s.handleSSE)
}

// ─── Page handlers ───────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/apps", http.StatusTemporaryRedirect)
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.querier.ListApplications(r.Context())
	if err != nil {
		apps = nil
	}
	html(w)
	_ = pages.AppsPage(apps, err).Render(r.Context(), w)
}

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	app, err := s.querier.GetApplication(r.Context(), name)
	if err != nil {
		http.Error(w, fmt.Sprintf("application %q not found: %v", name, err), http.StatusNotFound)
		return
	}
	domains, _ := s.querier.ListDomainsByApp(r.Context(), name)
	html(w)
	_ = pages.AppDetailPage(app, domains).Render(r.Context(), w)
}

func (s *Server) handleSandboxes(w http.ResponseWriter, r *http.Request) {
	app := r.URL.Query().Get("app")
	sandboxes, err := s.querier.ListAllSandboxes(r.Context(), app)
	if err != nil {
		sandboxes = nil
	}
	html(w)
	_ = pages.SandboxesPage(sandboxes, app, err).Render(r.Context(), w)
}

func (s *Server) handlePromotions(w http.ResponseWriter, r *http.Request) {
	filters := query.PromotionFilter{
		App:    r.URL.Query().Get("app"),
		Domain: r.URL.Query().Get("domain"),
		Status: r.URL.Query().Get("status"),
	}
	promotions, err := s.querier.ListPromotionRequests(r.Context(), filters)
	if err != nil {
		promotions = nil
	}
	html(w)
	_ = pages.PromotionsPage(promotions, filters, err).Render(r.Context(), w)
}

func (s *Server) handlePromotionApprove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	approver := r.FormValue("approver")
	role := r.FormValue("role")
	if approver == "" {
		approver = s.defaultUser
	}
	if role == "" {
		role = "org-admin"
	}

	pr := &choristerv1alpha1.ChoPromotionRequest{}
	if err := s.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: controlPlaneNamespace}, pr); err != nil {
		http.Error(w, fmt.Sprintf("promotion request not found: %v", err), http.StatusNotFound)
		return
	}
	if pr.Status.Phase != "" && pr.Status.Phase != "Pending" {
		http.Error(w, fmt.Sprintf("promotion request is in phase %q (must be Pending)", pr.Status.Phase), http.StatusConflict)
		return
	}

	pr.Status.Approvals = append(pr.Status.Approvals, choristerv1alpha1.PromotionApproval{
		Approver:   approver,
		Role:       role,
		ApprovedAt: metav1.Now(),
	})
	if err := s.client.Status().Update(r.Context(), pr); err != nil {
		http.Error(w, fmt.Sprintf("update promotion request: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/promotions", http.StatusSeeOther)
}

func (s *Server) handlePromotionReject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	pr := &choristerv1alpha1.ChoPromotionRequest{}
	if err := s.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: controlPlaneNamespace}, pr); err != nil {
		http.Error(w, fmt.Sprintf("promotion request not found: %v", err), http.StatusNotFound)
		return
	}
	if pr.Status.Phase != "" && pr.Status.Phase != "Pending" {
		http.Error(w, fmt.Sprintf("promotion request is in phase %q (must be Pending)", pr.Status.Phase), http.StatusConflict)
		return
	}

	pr.Status.Phase = "Rejected"
	upsertCondition(&pr.Status.Conditions, metav1.Condition{
		Type:               "Rejected",
		Status:             metav1.ConditionTrue,
		Reason:             "ManuallyRejected",
		Message:            "Rejected via Chorister web UI",
		ObservedGeneration: pr.Generation,
	})
	if err := s.client.Status().Update(r.Context(), pr); err != nil {
		http.Error(w, fmt.Sprintf("update promotion request: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/promotions", http.StatusSeeOther)
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	filters := query.MemberFilter{
		App:            r.URL.Query().Get("app"),
		Domain:         r.URL.Query().Get("domain"),
		IncludeExpired: r.URL.Query().Get("include_expired") == "true",
	}
	members, err := s.querier.ListMemberships(r.Context(), filters)
	if err != nil {
		members = nil
	}
	html(w)
	_ = pages.MembersPage(members, err).Render(r.Context(), w)
}

func (s *Server) handleVulns(w http.ResponseWriter, r *http.Request) {
	filters := query.VulnFilter{
		App:    r.URL.Query().Get("app"),
		Domain: r.URL.Query().Get("domain"),
	}
	vulns, err := s.querier.ListVulnerabilityReports(r.Context(), filters)
	if err != nil {
		vulns = nil
	}
	html(w)
	_ = pages.VulnsPage(vulns, err).Render(r.Context(), w)
}

func (s *Server) handleEventsPage(w http.ResponseWriter, r *http.Request) {
	// Render with last 50 events for the initial load; live updates come via SSE.
	events, _ := s.querier.ListChoristerEvents(r.Context(), controlPlaneNamespace, time.Hour, 50)
	html(w)
	_ = pages.EventsPage(events).Render(r.Context(), w)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// html sets the Content-Type header for HTML responses.
func html(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// upsertCondition upserts a metav1.Condition in the given slice.
func upsertCondition(conditions *[]metav1.Condition, c metav1.Condition) {
	if c.LastTransitionTime.IsZero() {
		c.LastTransitionTime = metav1.Now()
	}
	for i, existing := range *conditions {
		if existing.Type == c.Type {
			(*conditions)[i] = c
			return
		}
	}
	*conditions = append(*conditions, c)
}
