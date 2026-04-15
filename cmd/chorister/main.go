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

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
	"github.com/chorister-dev/chorister/internal/report"
)

var (
	// Set via -ldflags at build time.
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "chorister",
		Short: "Opinionated infrastructure platform for Kubernetes",
		Long: `chorister is an opinionated infrastructure platform that runs as a K8s operator.
It provides sandbox-first workflow, deterministic promotion, and compliance built in.`,
		SilenceUsage: true,
	}

	root.AddCommand(
		newVersionCmd(),
		newSetupCmd(),
		newLoginCmd(),
		newApplyCmd(),
		newSandboxCmd(),
		newDiffCmd(),
		newStatusCmd(),
		newPromoteCmd(),
		newApproveCmd(),
		newRejectCmd(),
		newRequestsCmd(),
		newLogsCmd(),
		newEventsCmd(),
		newAdminCmd(),
		newExportCmd(),
		newGetCmd(),
		newWaitCmd(),
		newDocsCmd(root),
	)

	return root
}

// --- version ---

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print build information",
		Example: "  chorister version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("chorister %s (commit: %s, built: %s)\n", version, commit, date)
		},
	}
}

// --- setup ---

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Bootstrap chorister controller and CRDs into the cluster",
		Long:  `Installs the controller Deployment and CRDs into cho-system namespace, then creates a default ChoCluster CRD to trigger stack bootstrap. Idempotent: running twice is safe.`,
		Example: `  chorister setup
  chorister setup --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: would create namespace cho-system")
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: would install CRDs into the cluster")
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: would deploy chorister controller manager")
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: would create default ChoCluster to bootstrap operator stack")
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "setup: requires a running cluster (use --dry-run to preview)")
			return fmt.Errorf("setup requires a running Kubernetes cluster. Set KUBECONFIG or run from within a cluster")
		},
	}
	cmd.Flags().Bool("dry-run", false, "Print what would be done without making changes")
	return cmd
}

// --- login ---

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "login",
		Short:   "Authenticate via OIDC",
		Long:    `Initiates an OIDC device-flow authentication and stores credentials for subsequent CLI calls. The OIDC provider is configured in the ChoCluster resource.`,
		Example: "  chorister login",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("login: not yet implemented")
			return nil
		},
	}
}

// --- apply ---

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply DSL to a sandbox (always sandbox, never production)",
		Long:  `Reads the DSL file and creates/updates CRDs in the target sandbox namespace. Refuses to target production namespaces. Use 'chorister promote' to move changes to production.`,
		Example: `  chorister apply --domain payments --sandbox alice --file infra.cho
  chorister apply -d payments -s alice -f infra.cho`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")

			if sandbox == "" {
				return fmt.Errorf("--sandbox is required: chorister apply always targets a sandbox, not production")
			}
			if domain == "" {
				return fmt.Errorf("--domain is required")
			}

			// Reject any sandbox name that looks like a production target
			if sandbox == "production" || sandbox == "prod" {
				return fmt.Errorf("cannot apply to production: sandbox name %q is a reserved production identifier. Use `chorister promote` to promote sandbox changes to production", sandbox)
			}

			fmt.Printf("apply: targeting domain=%s sandbox=%s (not yet implemented)\n", domain, sandbox)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Target sandbox name")
	cmd.Flags().StringP("file", "f", "", "DSL file to apply")
	return cmd
}

// --- sandbox ---

func newSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Manage sandbox lifecycle",
		Long:  `Create, list, inspect, and destroy sandboxes. Sandboxes are isolated namespaces for development and testing. All 'chorister apply' changes target a sandbox — use 'chorister promote' to push to production.`,
	}

	cmd.AddCommand(
		newSandboxCreateCmd(),
		newSandboxDestroyCmd(),
		newSandboxListCmd(),
		newSandboxStatusCmd(),
	)
	return cmd
}

func newSandboxCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new sandbox",
		Long:  `Provisions a new isolated namespace for development. The sandbox inherits the domain's resource limits and network policies but is fully isolated from production.`,
		Example: `  chorister sandbox create --domain payments --name alice
  chorister sandbox create --domain payments --name feature-x --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			name, _ := cmd.Flags().GetString("name")
			app, _ := cmd.Flags().GetString("app")
			owner, _ := cmd.Flags().GetString("owner")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			if owner == "" {
				owner = "cli-user"
			}

			sb := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: app + "-" + domain + "-",
					Namespace:    "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: app,
					Domain:      domain,
					Name:        name,
					Owner:       owner,
				},
			}
			if err := c.Create(cmd.Context(), sb); err != nil {
				return fmt.Errorf("create ChoSandbox: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sandbox %q created for domain %s in application %s (namespace will be provisioned by controller)\n", name, domain, app)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("name", "n", "", "Sandbox name")
	cmd.Flags().String("app", "", "Application name")
	cmd.Flags().String("owner", "", "Sandbox owner identity (defaults to cli-user)")
	return cmd
}

func newSandboxDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy a sandbox and all its resources",
		Long:  `Permanently deletes a sandbox namespace and all resources within it. This action is irreversible. Stateful resources (databases, queues) will be deleted without archiving.`,
		Example: `  chorister sandbox destroy --domain payments --name alice
  chorister sandbox destroy --domain payments --name alice --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			name, _ := cmd.Flags().GetString("name")
			app, _ := cmd.Flags().GetString("app")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			var list choristerv1alpha1.ChoSandboxList
			if err := c.List(cmd.Context(), &list); err != nil {
				return fmt.Errorf("list ChoSandboxes: %w", err)
			}

			var target *choristerv1alpha1.ChoSandbox
			for i := range list.Items {
				sb := &list.Items[i]
				if sb.Spec.Application == app && sb.Spec.Domain == domain && sb.Spec.Name == name {
					target = sb
					break
				}
			}
			if target == nil {
				return fmt.Errorf("sandbox %q not found in domain %s of application %s", name, domain, app)
			}

			if err := c.Delete(cmd.Context(), target); err != nil {
				return fmt.Errorf("delete ChoSandbox: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sandbox %q deleted from domain %s in application %s (controller will clean up namespace)\n", name, domain, app)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("name", "n", "", "Sandbox name")
	cmd.Flags().String("app", "", "Application name")
	return cmd
}

func newSandboxListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sandboxes",
		Long:  `Lists all sandboxes with owner, domain, age, last-apply time, estimated cost, and idle warning. Filter by domain or application.`,
		Example: `  chorister sandbox list
  chorister sandbox list --domain payments
  chorister sandbox list --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			app, _ := cmd.Flags().GetString("app")

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			var sandboxes []query.SandboxInfo
			if domain != "" {
				// Resolve app
				if app == "" {
					apps, qerr := q.ListApplications(cmd.Context())
					if qerr != nil {
						return qerr
					}
					if len(apps) == 1 {
						app = apps[0].Name
					} else {
						return fmt.Errorf("--app is required when multiple applications exist")
					}
				}
				sandboxes, err = q.ListSandboxesByDomain(cmd.Context(), app, domain)
			} else {
				sandboxes, err = q.ListAllSandboxes(cmd.Context(), app)
			}
			if err != nil {
				return err
			}

			td := report.SandboxListReport(sandboxes)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, sandboxes, &td)
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Filter by domain")
	cmd.Flags().String("app", "", "Filter by application name")
	addOutputFlag(cmd)
	return cmd
}

// --- diff ---

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare sandbox vs production",
		Long:  `Shows resource-level differences between sandbox and production namespaces: added, changed, removed resources.`,
		Example: `  chorister diff --domain payments --sandbox alice
  chorister diff --domain payments --sandbox alice --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")
			app, _ := cmd.Flags().GetString("app")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if sandbox == "" {
				return fmt.Errorf("--sandbox is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Resolve app
			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "diff: app=%s domain=%s sandbox=%s (not yet implemented — awaiting compilation integration)\n", app, domain, sandbox)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Source sandbox name")
	cmd.Flags().String("app", "", "Application name")
	addOutputFlag(cmd)
	return cmd
}

// --- status ---

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [domain]",
		Short: "Show domain health across environments",
		Long: `Show health summary for all domains in an application, or detailed status for a specific domain.

Without a domain argument, lists all domains with their phase, resource counts, and isolation state.
With a domain argument, shows production resource health and all active sandboxes.`,
		Example: `  # Show all domains for a single-app cluster
  chorister status

  # Show all domains for a specific application
  chorister status --app myproduct

  # Show detailed status for the payments domain
  chorister status payments --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			appName, _ := cmd.Flags().GetString("app")
			format := getOutputFormat(cmd)

			// If a specific domain is given, show domain detail + sandboxes
			if len(args) > 0 {
				domainName := args[0]

				// Resolve app if not provided
				if appName == "" {
					apps, err := q.ListApplications(cmd.Context())
					if err != nil {
						return err
					}
					if len(apps) == 1 {
						appName = apps[0].Name
					} else {
						return fmt.Errorf("--app is required when multiple applications exist")
					}
				}

				domains, err := q.ListDomainsByApp(cmd.Context(), appName)
				if err != nil {
					return err
				}
				var found *query.DomainInfo
				for i := range domains {
					if domains[i].Name == domainName {
						found = &domains[i]
						break
					}
				}
				if found == nil {
					return fmt.Errorf("domain %q not found in application %q", domainName, appName)
				}

				var resources *query.DomainResources
				if found.Namespace != "" {
					resources, err = q.ListDomainResources(cmd.Context(), found.Namespace)
					if err != nil {
						return err
					}
				} else {
					resources = &query.DomainResources{}
				}

				sandboxes, err := q.ListSandboxesByDomain(cmd.Context(), appName, domainName)
				if err != nil {
					return err
				}

				ss := report.DomainStatusReport(*found, resources, sandboxes)
				if format == "table" {
					renderStatusSummary(cmd.OutOrStdout(), &ss)
					if len(sandboxes) > 0 {
						fmt.Fprintln(cmd.OutOrStdout(), "")
						fmt.Fprintln(cmd.OutOrStdout(), "Sandboxes:")
						td := report.SandboxListReport(sandboxes)
						renderTable(cmd.OutOrStdout(), &td)
					}
					return nil
				}
				return renderOutput(cmd.OutOrStdout(), format, ss, nil)
			}

			// No domain arg: show summary for all domains
			if appName == "" {
				apps, err := q.ListApplications(cmd.Context())
				if err != nil {
					return err
				}
				if len(apps) == 1 {
					appName = apps[0].Name
				} else if len(apps) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No applications found")
					return nil
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			domains, err := q.ListDomainsByApp(cmd.Context(), appName)
			if err != nil {
				return err
			}

			td := report.DomainListReport(domains)
			return renderOutput(cmd.OutOrStdout(), format, domains, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name")
	addOutputFlag(cmd)
	return cmd
}

// --- promote ---

func newPromoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Create a ChoPromotionRequest to promote sandbox to production",
		Long: `Creates a ChoPromotionRequest that moves a sandbox's compiled resources into production.
The controller handles approval gating (if configured) and security scan requirements.

Use --rollback to revert production to its previous compiled state. Rollback and --sandbox are mutually exclusive.`,
		Example: `  chorister promote --domain payments --sandbox alice
  chorister promote --domain payments --rollback
  chorister promote --domain payments --sandbox alice --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")
			rollback, _ := cmd.Flags().GetBool("rollback")
			app, _ := cmd.Flags().GetString("app")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}

			if rollback && sandbox != "" {
				return fmt.Errorf("--rollback and --sandbox are mutually exclusive: rollback reverts production from its own history")
			}

			if !rollback && sandbox == "" {
				return fmt.Errorf("--sandbox is required (or use --rollback to revert production)")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Resolve app
			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-%s-", app, domain),
					Namespace:    "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: app,
					Domain:      domain,
					Sandbox:     sandbox,
					RequestedBy: "cli-user",
				},
			}

			if rollback {
				pr.ObjectMeta.GenerateName = fmt.Sprintf("%s-%s-rollback-", app, domain)
				// Sandbox is empty for rollback requests; controller interprets this as rollback
			}

			if err := c.Create(cmd.Context(), pr); err != nil {
				return fmt.Errorf("create ChoPromotionRequest: %w", err)
			}

			if rollback {
				fmt.Fprintf(cmd.OutOrStdout(), "Rollback ChoPromotionRequest created for domain %s\n", domain)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "ChoPromotionRequest created: domain=%s sandbox=%s\n", domain, sandbox)
			}
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Source sandbox name")
	cmd.Flags().String("app", "", "Application name")
	cmd.Flags().Bool("rollback", false, "Roll back production to previous state")
	return cmd
}

// --- approve ---

func newApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "approve [promotion-id]",
		Short:   "Approve a ChoPromotionRequest",
		Long:    `Records your approval on a pending ChoPromotionRequest. When the required number of approvals is reached, the controller begins executing the promotion.`,
		Example: "  chorister approve myproduct-payments-abc123",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("approve: id=%s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

// --- reject ---

func newRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "reject [promotion-id]",
		Short:   "Reject a ChoPromotionRequest",
		Long:    `Permanently rejects a pending ChoPromotionRequest. The sandbox remains intact; a new promotion request must be created to try again.`,
		Example: "  chorister reject myproduct-payments-abc123",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("reject: id=%s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

// --- requests ---

func newRequestsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List promotion requests",
		Long:  `Lists ChoPromotionRequests with their current phase, approval count, and age. Supports filtering by application, domain, and status.`,
		Example: `  chorister requests
  chorister requests --status pending
  chorister requests --domain payments --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, _ := cmd.Flags().GetString("app")
			domain, _ := cmd.Flags().GetString("domain")
			status, _ := cmd.Flags().GetString("status")

			filters := query.PromotionFilter{
				App:    app,
				Domain: domain,
				Status: status,
			}

			promotions, err := q.ListPromotionRequests(cmd.Context(), filters)
			if err != nil {
				return err
			}

			td := report.PromotionListReport(promotions)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, promotions, &td)
		},
	}
	cmd.Flags().String("app", "", "Filter by application name")
	cmd.Flags().String("domain", "", "Filter by domain name")
	cmd.Flags().String("status", "", "Filter by status: pending, approved, rejected, all")
	addOutputFlag(cmd)
	return cmd
}

// --- admin ---

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Platform administration commands (org-admin only)",
		Long:  `Platform administration: manage applications, domains, members, compliance, FinOps, vulnerability reports, and cluster upgrades. These commands require org-admin role.`,
	}

	cmd.AddCommand(
		newAdminAppCmd(),
		newAdminDomainCmd(),
		newAdminClusterCmd(),
		newAdminMemberCmd(),
		newAdminComplianceCmd(),
		newAdminAuditCmd(),
		newAdminFinOpsCmd(),
		newAdminQuotasCmd(),
		newAdminVulnerabilitiesCmd(),
		newAdminScanCmd(),
		newAdminIsolateCmd(),
		newAdminUnisolateCmd(),
		newAdminResourceCmd(),
		newAdminUpgradeCmd(),
		newAdminExportConfigCmd(),
	)
	return cmd
}

func newAdminAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
		Long:  `List, inspect, create, and delete ChoApplication resources. An application is the top-level boundary that owns domains, policies, and quotas.`,
	}

	cmd.AddCommand(
		newAdminAppListCmd(),
		newAdminAppGetCmd(),
		newAdminAppDeleteCmd(),
		&cobra.Command{
			Use:   "create [name]",
			Short: "Create a ChoApplication",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("admin app create: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
		newAdminAppSetPolicyCmd(),
	)
	return cmd
}

func newAdminAppListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all applications",
		Example: `  chorister admin app list
  chorister admin app list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			apps, err := q.ListApplications(cmd.Context())
			if err != nil {
				return err
			}

			td := report.AppListReport(apps)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, apps, &td)
		},
	}
	addOutputFlag(cmd)
	return cmd
}

func newAdminAppGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show application details",
		Example: `  chorister admin app get myproduct
  chorister admin app get myproduct --output yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			app, err := q.GetApplication(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			domains, err := q.ListDomainsByApp(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			format := getOutputFormat(cmd)
			ss := report.AppDetailReport(app, domains)
			if format == "table" {
				renderStatusSummary(cmd.OutOrStdout(), &ss)
				if len(domains) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "")
					fmt.Fprintln(cmd.OutOrStdout(), "Domains:")
					td := report.DomainListReport(domains)
					renderTable(cmd.OutOrStdout(), &td)
				}
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, ss, nil)
		},
	}
	addOutputFlag(cmd)
	return cmd
}

func newAdminAppDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an application",
		Long:  `Deletes the ChoApplication and all owned domains. Use --dry-run to preview impact. Requires --confirm to proceed. The controller handles cascade deletion via owner references.`,
		Example: `  chorister admin app delete myproduct --dry-run
  chorister admin app delete myproduct --confirm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			app, err := q.GetApplication(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			domains, err := q.ListDomainsByApp(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			confirm, _ := cmd.Flags().GetBool("confirm")

			// Print impact summary
			fmt.Fprintf(cmd.OutOrStdout(), "Application: %s\n", app.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Domains to be deleted: %d\n", len(domains))
			for _, d := range domains {
				ns := d.Namespace
				if ns == "" {
					ns = "(not yet created)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s (namespace: %s)\n", d.Name, ns)
			}

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "\nDry run: no changes made")
				return nil
			}

			if !confirm {
				return fmt.Errorf("deletion requires --confirm flag. Review the resources above and re-run with --confirm")
			}

			if err := c.Delete(cmd.Context(), app); err != nil {
				return fmt.Errorf("delete ChoApplication %s: %w", app.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nChoApplication %s deleted. Controller will clean up owned resources.\n", app.Name)
			return nil
		},
	}
	cmd.Flags().Bool("confirm", false, "Confirm deletion")
	cmd.Flags().Bool("dry-run", false, "Show what would be deleted without making changes")
	return cmd
}

func newAdminAppSetPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-policy <name>",
		Short: "Update application policy",
		Long:  `Updates the policy for a ChoApplication. Only flags that are explicitly set are changed; unspecified fields retain their current values.`,
		Example: `  chorister admin app set-policy myproduct --compliance standard
  chorister admin app set-policy myproduct --required-approvers 2 --require-security-scan
  chorister admin app set-policy myproduct --max-idle-days 7 --archive-retention 90d`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			changed := false
			if cmd.Flags().Changed("compliance") {
				v, _ := cmd.Flags().GetString("compliance")
				app.Spec.Policy.Compliance = v
				changed = true
			}
			if cmd.Flags().Changed("required-approvers") {
				v, _ := cmd.Flags().GetInt("required-approvers")
				app.Spec.Policy.Promotion.RequiredApprovers = v
				changed = true
			}
			if cmd.Flags().Changed("require-security-scan") {
				v, _ := cmd.Flags().GetBool("require-security-scan")
				app.Spec.Policy.Promotion.RequireSecurityScan = v
				changed = true
			}
			if cmd.Flags().Changed("archive-retention") {
				v, _ := cmd.Flags().GetString("archive-retention")
				app.Spec.Policy.ArchiveRetention = v
				changed = true
			}
			if cmd.Flags().Changed("max-idle-days") {
				v, _ := cmd.Flags().GetInt("max-idle-days")
				if app.Spec.Policy.Sandbox == nil {
					app.Spec.Policy.Sandbox = &choristerv1alpha1.SandboxPolicy{}
				}
				app.Spec.Policy.Sandbox.MaxIdleDays = &v
				changed = true
			}

			if !changed {
				return fmt.Errorf("no policy flags specified; use --compliance, --required-approvers, --require-security-scan, --archive-retention, or --max-idle-days")
			}

			if err := c.Update(cmd.Context(), app); err != nil {
				return fmt.Errorf("update ChoApplication %s: %w", appName, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Policy updated for application %s\n", appName)
			return nil
		},
	}
	cmd.Flags().String("compliance", "", "Compliance profile: essential, standard, regulated")
	cmd.Flags().Int("required-approvers", 0, "Number of required promotion approvers")
	cmd.Flags().Bool("require-security-scan", false, "Gate promotion on security scan results")
	cmd.Flags().String("archive-retention", "", "Archive retention duration (e.g. 30d, 90d, 1y)")
	cmd.Flags().Int("max-idle-days", 0, "Maximum sandbox idle days before auto-destroy")
	return cmd
}

func newAdminDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage domains",
		Long:  `List, inspect, create, delete, and configure domains within applications. Domains are bounded contexts (DDD) that own a set of resources and enforce a sensitivity level.`,
	}

	cmd.AddCommand(
		newAdminDomainListCmd(),
		newAdminDomainGetCmd(),
		newAdminDomainDeleteCmd(),
		newAdminDomainSetSensitivityCmd(),
		&cobra.Command{
			Use:   "create [name]",
			Short: "Create a domain within an application",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("admin domain create: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
	)
	return cmd
}

func newAdminDomainListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List domains across applications",
		Example: `  chorister admin domain list
  chorister admin domain list --app myproduct
  chorister admin domain list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			appFilter, _ := cmd.Flags().GetString("app")

			domains, err := q.ListAllDomains(cmd.Context(), appFilter)
			if err != nil {
				return err
			}

			td := report.DomainListReport(domains)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, domains, &td)
		},
	}
	cmd.Flags().String("app", "", "Filter by application name")
	addOutputFlag(cmd)
	return cmd
}

func newAdminDomainGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show domain details",
		Example: `  chorister admin domain get payments --app myproduct
  chorister admin domain get payments --app myproduct --output yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			domains, err := q.ListDomainsByApp(cmd.Context(), appName)
			if err != nil {
				return err
			}

			// Find the requested domain
			var found *query.DomainInfo
			for i := range domains {
				if domains[i].Name == args[0] {
					found = &domains[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("domain %q not found in application %q", args[0], appName)
			}

			// Get resources if namespace exists
			var resources *query.DomainResources
			if found.Namespace != "" {
				resources, err = q.ListDomainResources(cmd.Context(), found.Namespace)
				if err != nil {
					return err
				}
				found.ResourceCount = resources.TotalCount()
			} else {
				resources = &query.DomainResources{}
			}

			format := getOutputFormat(cmd)
			ss := report.DomainDetailReport(*found, resources)
			if format == "table" {
				renderStatusSummary(cmd.OutOrStdout(), &ss)
				if resources.TotalCount() > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "")
					fmt.Fprintln(cmd.OutOrStdout(), "Resources:")
					td := report.DomainResourcesTable(resources)
					renderTable(cmd.OutOrStdout(), &td)
				}
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, ss, nil)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

func newAdminDomainDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a domain from an application",
		Long:  `Removes the domain from ChoApplication.spec.domains. The controller handles namespace cleanup. Stateful resources enter the archive lifecycle. Use --dry-run to preview impact.`,
		Example: `  chorister admin domain delete payments --app myproduct --dry-run
  chorister admin domain delete payments --app myproduct --confirm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			domainName := args[0]

			// Verify domain exists
			var domainFound bool
			for _, d := range app.Spec.Domains {
				if d.Name == domainName {
					domainFound = true
					break
				}
			}
			if !domainFound {
				return fmt.Errorf("domain %q not found in application %q", domainName, appName)
			}

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			confirm, _ := cmd.Flags().GetBool("confirm")

			ns := ""
			if app.Status.DomainNamespaces != nil {
				ns = app.Status.DomainNamespaces[domainName]
			}

			// Print impact
			fmt.Fprintf(cmd.OutOrStdout(), "Domain: %s\n", domainName)
			fmt.Fprintf(cmd.OutOrStdout(), "Application: %s\n", appName)
			if ns != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Namespace: %s\n", ns)

				resources, resErr := q.ListDomainResources(cmd.Context(), ns)
				if resErr == nil && resources.TotalCount() > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Resources in namespace: %d\n", resources.TotalCount())
					if len(resources.Databases) > 0 || len(resources.Queues) > 0 || len(resources.Storages) > 0 {
						fmt.Fprintln(cmd.OutOrStdout(), "⚠ Warning: stateful resources (databases, queues, storage) will enter archive lifecycle")
					}
				}
			}

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "\nDry run: no changes made")
				return nil
			}

			if !confirm {
				return fmt.Errorf("deletion requires --confirm flag. Review the impact above and re-run with --confirm")
			}

			// Remove domain from application spec
			newDomains := make([]choristerv1alpha1.DomainSpec, 0, len(app.Spec.Domains)-1)
			for _, d := range app.Spec.Domains {
				if d.Name != domainName {
					newDomains = append(newDomains, d)
				}
			}
			app.Spec.Domains = newDomains

			if err := c.Update(cmd.Context(), app); err != nil {
				return fmt.Errorf("update ChoApplication %s: %w", appName, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nDomain %s removed from application %s. Controller will handle cleanup.\n", domainName, appName)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().Bool("confirm", false, "Confirm deletion")
	cmd.Flags().Bool("dry-run", false, "Show what would be deleted without making changes")
	return cmd
}

func newAdminMemberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage domain memberships",
		Long:  `Add, remove, list, and audit ChoDomainMembership resources. Memberships grant RBAC roles within a domain. Restricted domains and regulated applications require an expiry date.`,
	}

	cmd.AddCommand(
		newAdminMemberAddCmd(),
		newAdminMemberListCmd(),
		newAdminMemberAuditCmd(),
		&cobra.Command{
			Use:   "remove",
			Short: "Remove a member from a domain",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("admin member remove: (not yet implemented)")
				return nil
			},
		},
	)
	return cmd
}

func newAdminMemberAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a member to a domain",
		Long:  `Creates a ChoDomainMembership granting the identity a role in the specified domain. For restricted domains or regulated applications, --expires-at is required.`,
		Example: `  chorister admin member add --app myproduct --domain payments --identity alice@corp.com --role developer
  chorister admin member add --app myproduct --domain hr --identity bob@corp.com --role viewer --expires-at 2026-12-31T00:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			domainName, _ := cmd.Flags().GetString("domain")
			identity, _ := cmd.Flags().GetString("identity")
			role, _ := cmd.Flags().GetString("role")
			expiresAt, _ := cmd.Flags().GetString("expires-at")
			source, _ := cmd.Flags().GetString("source")

			if appName == "" {
				return fmt.Errorf("--app is required")
			}
			if domainName == "" {
				return fmt.Errorf("--domain is required")
			}
			if identity == "" {
				return fmt.Errorf("--identity is required")
			}
			if role == "" {
				return fmt.Errorf("--role is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Validate: check if domain is restricted → require --expires-at
			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return fmt.Errorf("get application %q: %w", appName, err)
			}

			isRestricted := false
			for _, d := range app.Spec.Domains {
				if d.Name == domainName {
					if d.Sensitivity == "restricted" {
						isRestricted = true
					}
					break
				}
			}

			isRegulated := app.Spec.Policy.Compliance == "regulated"
			if (isRestricted || isRegulated) && expiresAt == "" {
				if isRestricted {
					return fmt.Errorf("--expires-at is required for restricted domain %q", domainName)
				}
				return fmt.Errorf("--expires-at is required for regulated application %q", appName)
			}

			// Build the ChoDomainMembership
			membership := &choristerv1alpha1.ChoDomainMembership{}
			membership.GenerateName = appName + "-" + domainName + "-"
			membership.Namespace = "default"
			membership.Spec.Application = appName
			membership.Spec.Domain = domainName
			membership.Spec.Identity = identity
			membership.Spec.Role = role
			if source != "" {
				membership.Spec.Source = source
			}

			if expiresAt != "" {
				t, err := time.Parse(time.RFC3339, expiresAt)
				if err != nil {
					return fmt.Errorf("--expires-at must be in RFC3339 format (e.g. 2026-12-31T00:00:00Z): %w", err)
				}
				membership.Spec.ExpiresAt = &metav1.Time{Time: t}
			}

			if err := c.Create(cmd.Context(), membership); err != nil {
				return fmt.Errorf("create ChoDomainMembership: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Membership added: %s as %s in %s/%s\n", identity, role, appName, domainName)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().String("domain", "", "Domain name (required)")
	cmd.Flags().String("identity", "", "User identity / email (required)")
	cmd.Flags().String("role", "", "Role: org-admin, domain-admin, developer, viewer (required)")
	cmd.Flags().String("expires-at", "", "Expiry date in RFC3339 format (required for restricted/regulated)")
	cmd.Flags().String("source", "manual", "Source: manual or oidc-group")
	return cmd
}

func newAdminMemberListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List domain members",
		Example: `  chorister admin member list --app myproduct
  chorister admin member list --app myproduct --domain payments --role developer
  chorister admin member list --app myproduct --include-expired`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			appName, _ := cmd.Flags().GetString("app")
			domainName, _ := cmd.Flags().GetString("domain")
			role, _ := cmd.Flags().GetString("role")
			includeExpired, _ := cmd.Flags().GetBool("include-expired")

			members, err := q.ListMemberships(cmd.Context(), query.MemberFilter{
				App:            appName,
				Domain:         domainName,
				Role:           role,
				IncludeExpired: includeExpired,
			})
			if err != nil {
				return err
			}

			td := report.MemberListReport(members)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, members, &td)
		},
	}
	cmd.Flags().String("app", "", "Filter by application name")
	cmd.Flags().String("domain", "", "Filter by domain name")
	cmd.Flags().String("role", "", "Filter by role")
	cmd.Flags().Bool("include-expired", false, "Include expired memberships")
	addOutputFlag(cmd)
	return cmd
}

func newAdminMemberAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit memberships for stale or expired access",
		Long:  `Scans all memberships in the application and flags: expired memberships, memberships on restricted domains missing an expiry, and stale accounts with no recent activity.`,
		Example: `  chorister admin member audit --app myproduct
  chorister admin member audit --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			result, err := report.MembershipAuditReport(cmd.Context(), q, appName)
			if err != nil {
				return err
			}

			td := report.MemberAuditTableReport(result)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, result, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

func newAdminComplianceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance reporting",
		Long:  `Generate compliance reports and status summaries. Checks are mapped to CIS Controls, SOC 2, and ISO 27001 controls based on the application's compliance profile (essential/standard/regulated).`,
	}
	cmd.AddCommand(
		newAdminComplianceReportCmd(),
		newAdminComplianceStatusCmd(),
	)
	return cmd
}

func newAdminComplianceReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Full compliance report for an application",
		Long:  `Aggregates Gatekeeper constraints, kube-bench results, Tetragon status, TLS enforcement, and encryption-at-rest validation into a per-control report mapped to compliance frameworks.`,
		Example: `  chorister admin compliance report --app myproduct
  chorister admin compliance report --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			result, err := report.ComplianceReport(cmd.Context(), q, app)
			if err != nil {
				return err
			}

			td := report.ComplianceCheckTableReport(result)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, result, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

func newAdminComplianceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Compliance status summary for an application",
		Example: `  chorister admin compliance status --app myproduct`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			result, err := report.ComplianceReport(cmd.Context(), q, app)
			if err != nil {
				return err
			}

			summary := report.ComplianceStatusSummary(result)
			format := getOutputFormat(cmd)
			if format == "table" {
				renderStatusSummary(cmd.OutOrStdout(), &summary)
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, summary, nil)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

func newAdminIsolateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "isolate [domain]",
		Short:   "Isolate a domain (tighten NetworkPolicy, freeze promotions)",
		Long:    `Sets the chorister.dev/isolate-<domain>=true annotation on the ChoApplication. The controller responds by tightening network policies and blocking new promotion requests for the domain. Use during incident response.`,
		Example: `  chorister admin isolate payments --app myproduct`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainName := args[0]
			app, _ := cmd.Flags().GetString("app")
			if app == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			choApp, err := q.GetApplication(cmd.Context(), app)
			if err != nil {
				return err
			}

			annotationKey := fmt.Sprintf("chorister.dev/isolate-%s", domainName)
			patch := client.MergeFrom(choApp.DeepCopy())
			if choApp.Annotations == nil {
				choApp.Annotations = make(map[string]string)
			}
			choApp.Annotations[annotationKey] = "true"
			if err := c.Patch(cmd.Context(), choApp, patch); err != nil {
				return fmt.Errorf("patch ChoApplication %s: %w", app, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s in application %s is now isolated.\n", domainName, app)
			fmt.Fprintf(cmd.OutOrStdout(), "Annotation %s=true set. The controller will tighten network policies and block promotions.\n", annotationKey)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name")
	return cmd
}

func newAdminUnisolateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unisolate [domain]",
		Short:   "Restore a previously isolated domain",
		Long:    `Removes the chorister.dev/isolate-<domain> annotation, reverting network policies to their normal state and unblocking promotion requests.`,
		Example: `  chorister admin unisolate payments --app myproduct`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainName := args[0]
			app, _ := cmd.Flags().GetString("app")
			if app == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			choApp, err := q.GetApplication(cmd.Context(), app)
			if err != nil {
				return err
			}

			annotationKey := fmt.Sprintf("chorister.dev/isolate-%s", domainName)
			if choApp.Annotations == nil || choApp.Annotations[annotationKey] == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Domain %s in application %s is not currently isolated.\n", domainName, app)
				return nil
			}

			patch := client.MergeFrom(choApp.DeepCopy())
			delete(choApp.Annotations, annotationKey)
			if err := c.Patch(cmd.Context(), choApp, patch); err != nil {
				return fmt.Errorf("patch ChoApplication %s: %w", app, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s in application %s is now restored.\n", domainName, app)
			fmt.Fprintf(cmd.OutOrStdout(), "Annotation %s removed. The controller will revert network policies and unblock promotions.\n", annotationKey)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name")
	return cmd
}

func newAdminResourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources (e.g. list, delete archived)",
		Long:  `List all resources in a domain or delete resources that have entered the Archived/Deletable lifecycle state after domain deletion.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all resources in a domain",
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			domainName, _ := cmd.Flags().GetString("domain")
			archived, _ := cmd.Flags().GetBool("archived")

			if appName == "" {
				return fmt.Errorf("--app is required")
			}
			if domainName == "" {
				return fmt.Errorf("--domain is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Get domain namespace
			domains, err := q.ListDomainsByApp(cmd.Context(), appName)
			if err != nil {
				return err
			}
			var namespace string
			for _, d := range domains {
				if d.Name == domainName {
					namespace = d.Namespace
					break
				}
			}
			if namespace == "" {
				return fmt.Errorf("domain %q not found or has no namespace", domainName)
			}

			resources, err := q.ListDomainResources(cmd.Context(), namespace)
			if err != nil {
				return err
			}

			td := report.ResourceListReport(resources, archived)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, resources, &td)
		},
	}
	listCmd.Flags().String("app", "", "Application name (required)")
	listCmd.Flags().String("domain", "", "Domain name (required)")
	listCmd.Flags().Bool("archived", false, "Only show resources in Archived/Deletable lifecycle state")
	listCmd.Flags().String("type", "", "Filter by resource type: database, compute, queue, cache, storage")
	addOutputFlag(listCmd)
	cmd.AddCommand(listCmd)

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an archived resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			archived, _ := cmd.Flags().GetString("archived")
			if archived == "" {
				return fmt.Errorf("--archived <resource> is required. Usage: chorister admin resource delete --archived <resource-name> --type <database|queue|storage> --namespace <ns>")
			}

			resourceType, _ := cmd.Flags().GetString("type")
			if resourceType == "" {
				resourceType = "database" // default
			}

			namespace, _ := cmd.Flags().GetString("namespace")
			if namespace == "" {
				return fmt.Errorf("--namespace is required. Specify the production namespace containing the archived resource")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			obj, err := q.GetResource(cmd.Context(), resourceType, archived, namespace)
			if err != nil {
				return fmt.Errorf("get %s %q in namespace %q: %w", resourceType, archived, namespace, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleting archived %s %q in namespace %q\n", resourceType, archived, namespace)
			if err := c.Delete(cmd.Context(), obj); err != nil {
				return fmt.Errorf("delete %s %q: %w", resourceType, archived, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Archived %s %q deleted successfully.\n", resourceType, archived)
			return nil
		},
	}
	deleteCmd.Flags().String("archived", "", "Name of the archived resource to delete")
	deleteCmd.Flags().String("type", "database", "Resource type: database, queue, or storage")
	deleteCmd.Flags().String("namespace", "", "Namespace containing the resource")
	cmd.AddCommand(deleteCmd)

	return cmd
}

func newAdminUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Manage controller upgrades (blue-green)",
		Long: `Manage blue-green controller upgrades. Use --revision to deploy a new canary controller,
--promote to make a revision the stable default, or --rollback to remove a canary revision.

--revision sets the chorister.dev/canary-revision annotation on ChoCluster; the controller
deploys a canary instance alongside the stable one.
--promote updates spec.controllerRevision and clears the canary annotation.
--rollback removes the canary annotation without changing the stable revision.`,
		Example: `  chorister admin upgrade --revision v1.3.0
  chorister admin upgrade --promote v1.3.0
  chorister admin upgrade --rollback v1.3.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			revision, _ := cmd.Flags().GetString("revision")
			promote, _ := cmd.Flags().GetString("promote")
			rollback, _ := cmd.Flags().GetString("rollback")

			if revision == "" && promote == "" && rollback == "" {
				return fmt.Errorf("one of --revision, --promote, or --rollback is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			cluster, err := q.GetCluster(cmd.Context())
			if err != nil {
				return fmt.Errorf("get ChoCluster: %w", err)
			}

			patch := client.MergeFrom(cluster.DeepCopy())
			if cluster.Annotations == nil {
				cluster.Annotations = make(map[string]string)
			}

			switch {
			case revision != "":
				cluster.Annotations["chorister.dev/canary-revision"] = revision
				if err := c.Patch(cmd.Context(), cluster, patch); err != nil {
					return fmt.Errorf("set canary revision on ChoCluster: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deployed canary revision %q alongside stable %q.\n", revision, cluster.Spec.ControllerRevision)
				fmt.Fprintf(cmd.OutOrStdout(), "Use --promote %s to make it the stable default when ready.\n", revision)
			case promote != "":
				cluster.Spec.ControllerRevision = promote
				delete(cluster.Annotations, "chorister.dev/canary-revision")
				if err := c.Patch(cmd.Context(), cluster, patch); err != nil {
					return fmt.Errorf("promote revision on ChoCluster: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Revision %q is now the stable default (spec.controllerRevision updated).\n", promote)
			case rollback != "":
				canary := cluster.Annotations["chorister.dev/canary-revision"]
				if canary == "" {
					return fmt.Errorf("no canary revision is currently deployed")
				}
				if canary != rollback {
					return fmt.Errorf("canary revision is %q, not %q", canary, rollback)
				}
				delete(cluster.Annotations, "chorister.dev/canary-revision")
				if err := c.Patch(cmd.Context(), cluster, patch); err != nil {
					return fmt.Errorf("remove canary revision from ChoCluster: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Canary revision %q removed. Stable revision remains %q.\n", rollback, cluster.Spec.ControllerRevision)
			}
			return nil
		},
	}
	cmd.Flags().String("revision", "", "Deploy a new controller revision as canary")
	cmd.Flags().String("promote", "", "Promote a canary revision to stable")
	cmd.Flags().String("rollback", "", "Remove a canary revision")
	return cmd
}

// --- admin audit (CLI-6.1) ---

func newAdminAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query the audit log for platform events",
		Long: `Queries the Loki audit log for chorister platform events: promotions, approvals, member changes, isolation events, and resource deletions.

The Loki URL is read from the CHORISTER_LOKI_URL environment variable (default: http://loki.monitoring.svc:3100).`,
		Example: `  chorister admin audit --since 24h
  chorister admin audit --domain payments --actor alice@corp.com
  chorister admin audit --action promote --since 7d`,
		RunE: func(cmd *cobra.Command, args []string) error {
			lokiURL := os.Getenv("CHORISTER_LOKI_URL")
			if lokiURL == "" {
				lokiURL = "http://loki.monitoring.svc:3100"
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			domain, _ := cmd.Flags().GetString("domain")
			action, _ := cmd.Flags().GetString("action")
			actor, _ := cmd.Flags().GetString("actor")
			sinceStr, _ := cmd.Flags().GetString("since")

			since := 24 * time.Hour
			if sinceStr != "" {
				since, err = time.ParseDuration(sinceStr)
				if err != nil {
					return fmt.Errorf("--since: invalid duration %q (use e.g. 24h, 7d): %w", sinceStr, err)
				}
			}

			entries, err := q.QueryAuditLog(cmd.Context(), lokiURL, query.AuditFilter{
				Domain: domain,
				Action: action,
				Actor:  actor,
				Since:  since,
			})
			if err != nil {
				return err
			}

			td := report.AuditReport(entries)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, entries, &td)
		},
	}
	cmd.Flags().String("domain", "", "Filter by domain name")
	cmd.Flags().String("action", "", "Filter by action type (e.g. create, delete, promote)")
	cmd.Flags().String("actor", "", "Filter by actor email")
	cmd.Flags().String("since", "24h", "Show events since (e.g. 24h, 7d)")
	cmd.Flags().Int("limit", 100, "Maximum number of audit entries to return")
	addOutputFlag(cmd)
	return cmd
}

// --- admin finops (CLI-7.1, CLI-7.2) ---

func newAdminFinOpsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finops",
		Short: "FinOps cost reporting and budget tracking",
		Long:  `Cost reporting and budget tracking across domains and sandboxes. Rates are sourced from ChoCluster.spec.rates. Sandbox costs include idle-warning detection.`,
	}
	cmd.AddCommand(
		newAdminFinOpsReportCmd(),
		newAdminFinOpsBudgetCmd(),
	)
	return cmd
}

func newAdminFinOpsReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Cost breakdown by domain and sandbox",
		Example: `  chorister admin finops report --app myproduct
  chorister admin finops report --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			result, err := report.FinOpsReport(cmd.Context(), q, appName)
			if err != nil {
				return err
			}

			td := report.FinOpsTableReport(result)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, result, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

func newAdminFinOpsBudgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Budget utilization per domain",
		Example: `  chorister admin finops budget --app myproduct
  chorister admin finops budget --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			result, err := report.BudgetReport(cmd.Context(), q, appName)
			if err != nil {
				return err
			}

			td := report.BudgetTableReport(result)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, result, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

// --- admin quotas (CLI-7.3) ---

func newAdminQuotasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quotas",
		Short: "Resource quota utilization per domain",
		Long:  `Shows CPU, memory, storage, and pod count utilization versus ResourceQuota limits for each domain namespace.`,
		Example: `  chorister admin quotas --app myproduct
  chorister admin quotas --app myproduct --domain payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			domainFilter, _ := cmd.Flags().GetString("domain")

			result, err := report.QuotaReport(cmd.Context(), q, appName, domainFilter)
			if err != nil {
				return err
			}

			td := report.QuotaTableReport(result)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, result, &td)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().String("domain", "", "Filter to a specific domain")
	addOutputFlag(cmd)
	return cmd
}

// --- admin domain set-sensitivity (CLI-8.3) ---

func newAdminDomainSetSensitivityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-sensitivity <domain-name>",
		Short: "Set the sensitivity level for a domain",
		Long: `Updates a domain's sensitivity level. Sensitivity can be escalated but never reduced below the application's compliance baseline.

Sensitivity levels (ascending): public < internal < confidential < restricted
Compliance minimums: essential=public, standard=internal, regulated=confidential`,
		Example: `  chorister admin domain set-sensitivity payments --app myproduct --level confidential
  chorister admin domain set-sensitivity hr --app myproduct --level restricted`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainName := args[0]
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}
			level, _ := cmd.Flags().GetString("level")
			if level == "" {
				return fmt.Errorf("--level is required (public|internal|confidential|restricted)")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			// Validate sensitivity escalation rules
			sensitivityRank := map[string]int{
				"public": 0, "internal": 1, "confidential": 2, "restricted": 3,
			}
			complianceMinSensitivity := map[string]string{
				"essential": "public", "standard": "internal", "regulated": "confidential",
			}
			minLevel := complianceMinSensitivity[app.Spec.Policy.Compliance]
			if sensitivityRank[level] < sensitivityRank[minLevel] {
				return fmt.Errorf("sensitivity level %q is below minimum %q for compliance profile %q", level, minLevel, app.Spec.Policy.Compliance)
			}

			// Find and update the domain sensitivity
			found := false
			for i := range app.Spec.Domains {
				if app.Spec.Domains[i].Name == domainName {
					app.Spec.Domains[i].Sensitivity = level
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("domain %q not found in application %q", domainName, appName)
			}

			if err := c.Update(cmd.Context(), app); err != nil {
				return fmt.Errorf("update application: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Domain %q sensitivity set to %q in application %q\n", domainName, level, appName)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().String("level", "", "Sensitivity level: public, internal, confidential, restricted (required)")
	return cmd
}

// --- admin export-config (CLI-10.2) ---

func newAdminExportConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export-config",
		Short: "Export application configuration as CRD YAML for backup or migration",
		Long:  `Exports ChoApplication and ChoDomainMembership CRDs as YAML files to a local directory. Use for disaster recovery, cluster migration, or configuration audits.`,
		Example: `  chorister admin export-config --app myproduct
  chorister admin export-config --app myproduct --output-dir ./backup/2026-04-15`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, _ := cmd.Flags().GetString("app")
			if appName == "" {
				return fmt.Errorf("--app is required")
			}
			outputDir, _ := cmd.Flags().GetString("output-dir")
			if outputDir == "" {
				outputDir = filepath.Join(".", "backup")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, err := q.GetApplication(cmd.Context(), appName)
			if err != nil {
				return err
			}

			// Create output directory
			if err := os.MkdirAll(outputDir, 0750); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}

			// Export ChoApplication
			appFile := filepath.Join(outputDir, appName+"-application.yaml")
			app.TypeMeta = metav1.TypeMeta{APIVersion: "chorister.dev/v1alpha1", Kind: "ChoApplication"}
			cleanExportMeta(&app.ObjectMeta)
			appBytes, err := yaml.Marshal(app)
			if err != nil {
				return fmt.Errorf("marshal application: %w", err)
			}
			if err := os.WriteFile(appFile, appBytes, 0600); err != nil {
				return fmt.Errorf("write application YAML: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exported: %s\n", appFile)

			// Export memberships - fetch full CRDs directly.
			var memberList choristerv1alpha1.ChoDomainMembershipList
			if lerr := c.List(cmd.Context(), &memberList); lerr != nil {
				return fmt.Errorf("list memberships: %w", lerr)
			}
			var activeMembers []choristerv1alpha1.ChoDomainMembership
			for _, m := range memberList.Items {
				if m.Spec.Application == appName {
					activeMembers = append(activeMembers, m)
				}
			}
			if len(activeMembers) > 0 {
				membersFile := filepath.Join(outputDir, appName+"-memberships.yaml")
				var allMemberBytes []byte
				for i, m := range activeMembers {
					m.TypeMeta = metav1.TypeMeta{APIVersion: "chorister.dev/v1alpha1", Kind: "ChoDomainMembership"}
					cleanExportMeta(&m.ObjectMeta)
					data, merr := yaml.Marshal(m)
					if merr != nil {
						return fmt.Errorf("marshal membership %s: %w", m.Name, merr)
					}
					if i > 0 {
						allMemberBytes = append(allMemberBytes, []byte("---\n")...)
					}
					allMemberBytes = append(allMemberBytes, data...)
				}
				if err := os.WriteFile(membersFile, allMemberBytes, 0600); err != nil {
					return fmt.Errorf("write memberships YAML: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported: %s (%d membership(s))\n", membersFile, len(activeMembers))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Export complete: %s\n", outputDir)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().String("output-dir", "./backup", "Directory to write exported YAML files")
	return cmd
}

// --- admin cluster (CLI-2) ---

func newAdminClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster status and operator management",
		Long:  `Inspect ChoCluster status, operator health, CIS benchmark results, and observability stack readiness.`,
	}
	cmd.AddCommand(
		newAdminClusterStatusCmd(),
		newAdminClusterOperatorsCmd(),
	)
	return cmd
}

func newAdminClusterStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cluster status and operator health",
		Example: `  chorister admin cluster status
  chorister admin cluster status --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			cluster, err := q.GetCluster(cmd.Context())
			if err != nil {
				return err
			}

			ss := report.ClusterStatusReport(cluster)
			format := getOutputFormat(cmd)
			if format == "table" {
				renderStatusSummary(cmd.OutOrStdout(), &ss)
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, ss, nil)
		},
	}
	addOutputFlag(cmd)
	return cmd
}

func newAdminClusterOperatorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operators",
		Short: "List managed operators with version and health",
		Example: `  chorister admin cluster operators
  chorister admin cluster operators --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)
			operators, err := q.GetOperatorDetails(cmd.Context())
			if err != nil {
				return err
			}

			td := report.OperatorListReport(operators)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, operators, &td)
		},
	}
	addOutputFlag(cmd)
	return cmd
}

// --- sandbox status (CLI-3.3) ---

func newSandboxStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show detailed sandbox status",
		Long:  `Shows owner, age, estimated cost, last-apply time, idle warning, resource breakdown, and conditions for a specific sandbox.`,
		Example: `  chorister sandbox status --domain payments --name alice
  chorister sandbox status --domain payments --name alice --app myproduct --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			name, _ := cmd.Flags().GetString("name")
			app, _ := cmd.Flags().GetString("app")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Resolve app
			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			detail, err := q.GetSandbox(cmd.Context(), app, domain, name)
			if err != nil {
				return err
			}

			ss := report.SandboxDetailReport(detail)
			format := getOutputFormat(cmd)
			if format == "table" {
				renderStatusSummary(cmd.OutOrStdout(), &ss)
				if detail.Resources != nil && detail.Resources.TotalCount() > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "")
					fmt.Fprintln(cmd.OutOrStdout(), "Resources:")
					td := report.DomainResourcesTable(detail.Resources)
					renderTable(cmd.OutOrStdout(), &td)
				}
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, ss, nil)
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("name", "n", "", "Sandbox name")
	cmd.Flags().String("app", "", "Application name")
	addOutputFlag(cmd)
	return cmd
}

// --- logs (CLI-3.2) ---

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [component]",
		Short: "Stream logs from a sandbox component",
		Long: `Resolves the sandbox namespace from --domain and --sandbox, then streams logs from the matching pod.
Components are matched by the chorister.dev/component label.

Omit the component argument to list available components in the sandbox.
Always targets a sandbox — use kubectl for production log access.`,
		Example: `  # List available components in a sandbox
  chorister logs --domain payments --sandbox alice

  # Stream logs from the api component
  chorister logs api --domain payments --sandbox alice --follow

  # Show last 50 lines
  chorister logs api --domain payments --sandbox alice --tail 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")
			app, _ := cmd.Flags().GetString("app")

			if sandbox == "" {
				return fmt.Errorf("--sandbox is required: logs always target a sandbox. Use kubectl for production logs")
			}
			if domain == "" {
				return fmt.Errorf("--domain is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			if app == "" {
				apps, qerr := q.ListApplications(cmd.Context())
				if qerr != nil {
					return qerr
				}
				if len(apps) == 1 {
					app = apps[0].Name
				} else {
					return fmt.Errorf("--app is required when multiple applications exist")
				}
			}

			// Get sandbox namespace from ChoSandbox status.
			detail, err := q.GetSandbox(cmd.Context(), app, domain, sandbox)
			if err != nil {
				return fmt.Errorf("get sandbox: %w", err)
			}
			if detail.Namespace == "" {
				return fmt.Errorf("sandbox %q has no namespace yet (controller may still be provisioning)", sandbox)
			}
			ns := detail.Namespace

			cs, err := getKubeClientset(cmd)
			if err != nil {
				return err
			}

			follow, _ := cmd.Flags().GetBool("follow")
			tail, _ := cmd.Flags().GetInt64("tail")
			previous, _ := cmd.Flags().GetBool("previous")
			container, _ := cmd.Flags().GetString("container")

			// Build label selector: narrow to component if arg provided.
			labelSelector := "chorister.dev/component"
			if len(args) > 0 {
				labelSelector = "chorister.dev/component=" + args[0]
			}

			podList, err := cs.CoreV1().Pods(ns).List(cmd.Context(), metav1.ListOptions{
				LabelSelector: labelSelector,
			})
			if err != nil {
				return fmt.Errorf("list pods in %s: %w", ns, err)
			}

			// No component arg: list available components and exit.
			if len(args) == 0 {
				if len(podList.Items) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No components found in sandbox %q (namespace %s)\n", sandbox, ns)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Available components in sandbox %q:\n", sandbox)
				seen := make(map[string]bool)
				for _, pod := range podList.Items {
					comp := pod.Labels["chorister.dev/component"]
					if comp != "" && !seen[comp] {
						fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", comp)
						seen[comp] = true
					}
				}
				return nil
			}

			if len(podList.Items) == 0 {
				return fmt.Errorf("no pods found for component %q in sandbox %q (namespace %s)", args[0], sandbox, ns)
			}

			podName := podList.Items[0].Name
			logOpts := &corev1.PodLogOptions{
				Follow:    follow,
				Previous:  previous,
				Container: container,
				TailLines: &tail,
			}
			stream, err := cs.CoreV1().Pods(ns).GetLogs(podName, logOpts).Stream(cmd.Context())
			if err != nil {
				return fmt.Errorf("stream logs from pod %s: %w", podName, err)
			}
			defer stream.Close()

			_, err = io.Copy(cmd.OutOrStdout(), stream)
			return err
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Target sandbox name")
	cmd.Flags().String("app", "", "Application name")
	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().Int64("tail", 100, "Number of recent log lines to show")
	cmd.Flags().Bool("previous", false, "Show logs from previous container instance")
	cmd.Flags().String("container", "", "Container name")
	return cmd
}

// --- events (CLI-3.5) ---

func newEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List chorister-related Kubernetes events",
		Long:  `Lists Kubernetes Events related to chorister resources. Scope to a domain or sandbox to narrow results. Defaults to the last hour with a limit of 100 events.`,
		Example: `  chorister events
  chorister events --domain payments --since 24h
  chorister events --domain payments --sandbox alice`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")
			app, _ := cmd.Flags().GetString("app")
			sinceStr, _ := cmd.Flags().GetString("since")
			limit, _ := cmd.Flags().GetInt("limit")

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Resolve namespace from flags
			namespace := ""
			if domain != "" || sandbox != "" {
				if app == "" {
					apps, qerr := q.ListApplications(cmd.Context())
					if qerr != nil {
						return qerr
					}
					if len(apps) == 1 {
						app = apps[0].Name
					} else {
						return fmt.Errorf("--app is required when multiple applications exist")
					}
				}
				if sandbox != "" && domain != "" {
					namespace = app + "-" + domain + "-sbx-" + sandbox
				} else if domain != "" {
					namespace = app + "-" + domain
				}
			}

			since, err := parseDuration(sinceStr)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", sinceStr, err)
			}

			events, err := q.ListChoristerEvents(cmd.Context(), namespace, since, limit)
			if err != nil {
				return err
			}

			td := report.EventListReport(events)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, events, &td)
		},
	}
	cmd.Flags().String("domain", "", "Filter by domain")
	cmd.Flags().String("sandbox", "", "Filter by sandbox")
	cmd.Flags().String("app", "", "Application name")
	cmd.Flags().String("since", "1h", "Show events since duration (e.g. 1h, 24h)")
	cmd.Flags().Int("limit", 100, "Maximum number of events to show")
	addOutputFlag(cmd)
	return cmd
}

// --- admin vulnerabilities (CLI-5) ---

func newAdminVulnerabilitiesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vulnerabilities",
		Short: "Vulnerability report management",
		Long:  `List and inspect ChoVulnerabilityReport resources. Reports are generated by automated image scans and are used as a promotion gate for regulated applications.`,
	}
	cmd.AddCommand(
		newAdminVulnListCmd(),
		newAdminVulnGetCmd(),
	)
	return cmd
}

func newAdminVulnListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List vulnerability reports across domains",
		Example: `  chorister admin vulnerabilities list --app myproduct
  chorister admin vulnerabilities list --app myproduct --severity critical`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			app, _ := cmd.Flags().GetString("app")
			domain, _ := cmd.Flags().GetString("domain")
			severity, _ := cmd.Flags().GetString("severity")

			filters := query.VulnFilter{
				App:         app,
				Domain:      domain,
				MinSeverity: severity,
			}

			reports, err := q.ListVulnerabilityReports(cmd.Context(), filters)
			if err != nil {
				return err
			}

			td := report.VulnSummaryReport(reports)
			format := getOutputFormat(cmd)
			return renderOutput(cmd.OutOrStdout(), format, reports, &td)
		},
	}
	cmd.Flags().String("app", "", "Filter by application name")
	cmd.Flags().String("domain", "", "Filter by domain name")
	cmd.Flags().String("severity", "", "Minimum severity: critical, high, all")
	addOutputFlag(cmd)
	return cmd
}

func newAdminVulnGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <domain>",
		Short: "Show vulnerability report details for a domain",
		Example: `  chorister admin vulnerabilities get payments --app myproduct
  chorister admin vulnerabilities get payments --app myproduct --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, _ := cmd.Flags().GetString("app")
			if app == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			detail, err := q.GetVulnerabilityReport(cmd.Context(), app, args[0])
			if err != nil {
				return err
			}

			format := getOutputFormat(cmd)
			if format == "table" {
				// Print summary header
				fmt.Fprintf(cmd.OutOrStdout(), "Domain:   %s\n", detail.Domain)
				fmt.Fprintf(cmd.OutOrStdout(), "Scanner:  %s\n", detail.Scanner)
				fmt.Fprintf(cmd.OutOrStdout(), "Critical: %d\n", detail.CriticalCount)
				fmt.Fprintf(cmd.OutOrStdout(), "High:     %d\n", detail.HighCount)
				fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", detail.Phase)
				if len(detail.Findings) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "")
					fmt.Fprintln(cmd.OutOrStdout(), "Findings:")
					td := report.VulnDetailReport(detail)
					renderTable(cmd.OutOrStdout(), &td)
				}
				return nil
			}
			return renderOutput(cmd.OutOrStdout(), format, detail, nil)
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	addOutputFlag(cmd)
	return cmd
}

// --- admin scan (CLI-5.3) ---

func newAdminScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Trigger on-demand vulnerability scan",
		Long:  `Triggers an immediate vulnerability scan for one or all domains in an application by annotating the scan CronJob with kubectl.kubernetes.io/trigger-at. Results appear in ChoVulnerabilityReport.`,
		Example: `  chorister admin scan --app myproduct
  chorister admin scan --app myproduct --domain payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, _ := cmd.Flags().GetString("app")
			domainFilter, _ := cmd.Flags().GetString("domain")

			if app == "" {
				return fmt.Errorf("--app is required")
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			domains, err := q.ListDomainsByApp(cmd.Context(), app)
			if err != nil {
				return err
			}

			triggerTime := time.Now().UTC().Format(time.RFC3339)
			triggered := 0
			for _, d := range domains {
				if domainFilter != "" && d.Name != domainFilter {
					continue
				}
				if d.Namespace == "" {
					continue
				}

				cronJob := &batchv1.CronJob{}
				key := types.NamespacedName{Name: "vulnerability-scan", Namespace: d.Namespace}
				if err := c.Get(cmd.Context(), key, cronJob); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  Warning: no scan CronJob found in namespace %s (domain %s may not have compliance>=standard)\n", d.Namespace, d.Name)
					continue
				}

				patch := client.MergeFrom(cronJob.DeepCopy())
				if cronJob.Annotations == nil {
					cronJob.Annotations = make(map[string]string)
				}
				cronJob.Annotations["kubectl.kubernetes.io/trigger-at"] = triggerTime
				if err := c.Patch(cmd.Context(), cronJob, patch); err != nil {
					return fmt.Errorf("annotate CronJob in namespace %s: %w", d.Namespace, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Triggered vulnerability scan for domain %s (namespace %s)\n", d.Name, d.Namespace)
				triggered++
			}

			if triggered == 0 {
				if domainFilter != "" {
					return fmt.Errorf("domain %q not found or has no scan CronJob in application %s", domainFilter, app)
				}
				return fmt.Errorf("no scan CronJobs found in application %s (compliance must be standard or regulated)", app)
			}
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name (required)")
	cmd.Flags().String("domain", "", "Scan specific domain only")
	return cmd
}

// parseDuration parses a duration string like "1h", "24h", "30m".
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return time.Hour, nil
	}
	return time.ParseDuration(s)
}

// --- export ---

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export domain Cho CRDs as static YAML for GitOps",
		Long:  `Fetches all Cho CRDs from a live domain namespace and writes them as static YAML manifests to a directory. Suitable for backup, cluster migration, or GitOps workflows.`,
		Example: `  chorister export --domain payments
  chorister export --domain payments --app myproduct --output ./gitops/payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			appName, _ := cmd.Flags().GetString("app")
			output, _ := cmd.Flags().GetString("output")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			return runExport(cmd, appName, domain, output)
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().String("app", "", "Application name")
	cmd.Flags().StringP("output", "o", "./export", "Output directory")
	return cmd
}

func runExport(cmd *cobra.Command, appName, domain, outputDir string) error {
	c, err := getClient(cmd)
	if err != nil {
		return err
	}
	q := query.NewQuerier(c)

	// Resolve app.
	if appName == "" {
		apps, qerr := q.ListApplications(cmd.Context())
		if qerr != nil {
			return qerr
		}
		if len(apps) == 1 {
			appName = apps[0].Name
		} else {
			return fmt.Errorf("--app is required when multiple applications exist")
		}
	}

	// Find domain namespace from ChoApplication status.
	choApp, err := q.GetApplication(cmd.Context(), appName)
	if err != nil {
		return err
	}
	ns := ""
	if choApp.Status.DomainNamespaces != nil {
		ns = choApp.Status.DomainNamespaces[domain]
	}
	if ns == "" {
		return fmt.Errorf("domain %q in application %q has no namespace (controller may not have reconciled it yet)", domain, appName)
	}

	// List all Cho CRDs in the domain namespace.
	resources, err := q.ListDomainResources(cmd.Context(), ns)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	outputPath := filepath.Join(outputDir, domain+".yaml")
	var buf []byte
	sep := []byte("---\n")

	const apiVersion = "chorister.dev/v1alpha1"
	appendYAML := func(kind string, obj interface{}) error {
		data, merr := yaml.Marshal(obj)
		if merr != nil {
			return fmt.Errorf("marshal %s: %w", kind, merr)
		}
		if len(buf) > 0 {
			buf = append(buf, sep...)
		}
		buf = append(buf, data...)
		return nil
	}

	for i := range resources.Computes {
		r := resources.Computes[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoCompute"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoCompute", r); err != nil {
			return err
		}
	}
	for i := range resources.Databases {
		r := resources.Databases[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoDatabase"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoDatabase", r); err != nil {
			return err
		}
	}
	for i := range resources.Queues {
		r := resources.Queues[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoQueue"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoQueue", r); err != nil {
			return err
		}
	}
	for i := range resources.Caches {
		r := resources.Caches[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoCache"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoCache", r); err != nil {
			return err
		}
	}
	for i := range resources.Storages {
		r := resources.Storages[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoStorage"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoStorage", r); err != nil {
			return err
		}
	}
	for i := range resources.Networks {
		r := resources.Networks[i]
		r.TypeMeta = metav1.TypeMeta{APIVersion: apiVersion, Kind: "ChoNetwork"}
		cleanExportMeta(&r.ObjectMeta)
		if err := appendYAML("ChoNetwork", r); err != nil {
			return err
		}
	}

	if len(buf) == 0 {
		buf = []byte(fmt.Sprintf("# chorister export: app=%s domain=%s namespace=%s\n# No Cho CRDs found in domain namespace.\n", appName, domain, ns))
	}

	if err := os.WriteFile(outputPath, buf, 0o600); err != nil {
		return fmt.Errorf("write export: %w", err)
	}

	total := resources.TotalCount()
	fmt.Fprintf(cmd.OutOrStdout(), "Exported %d resource(s) from domain %s (namespace %s) to %s\n", total, domain, ns, outputPath)
	return nil
}

// cleanExportMeta strips runtime-assigned fields that should not be re-applied.
func cleanExportMeta(meta *metav1.ObjectMeta) {
	meta.ResourceVersion = ""
	meta.UID = ""
	meta.Generation = 0
	meta.ManagedFields = nil
}

// --- get <type> <name> (CLI-8.2) ---

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <type> <name>",
		Short: "Inspect a chorister resource (compute, database, queue, cache, storage, network, sandbox, promotion)",
		Long: `Shows the full spec and status of a specific chorister resource.

Resource types: compute, database, queue, cache, storage, network, sandbox, promotion

Use --app and --domain to resolve the namespace automatically, or pass --namespace directly.`,
		Example: `  chorister get database ledger --domain payments --app myproduct
  chorister get compute api --domain payments --app myproduct --output yaml
  chorister get sandbox alice --namespace myproduct-payments-sbx-alice`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := args[0]
			name := args[1]
			namespace, _ := cmd.Flags().GetString("namespace")
			appName, _ := cmd.Flags().GetString("app")
			domainName, _ := cmd.Flags().GetString("domain")

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			// Resolve namespace from domain if not given directly
			if namespace == "" && appName != "" && domainName != "" {
				domains, err := q.ListDomainsByApp(cmd.Context(), appName)
				if err != nil {
					return fmt.Errorf("list domains: %w", err)
				}
				for _, d := range domains {
					if d.Name == domainName {
						namespace = d.Namespace
						break
					}
				}
			}

			obj, err := q.GetResource(cmd.Context(), kind, name, namespace)
			if err != nil {
				return err
			}

			format := getOutputFormat(cmd)
			if format == "" {
				format = "yaml"
			}
			return renderOutput(cmd.OutOrStdout(), format, obj, nil)
		},
	}
	cmd.Flags().String("namespace", "", "Kubernetes namespace (alternative to --app + --domain)")
	cmd.Flags().String("app", "", "Application name (used with --domain to resolve namespace)")
	cmd.Flags().String("domain", "", "Domain name (used with --app to resolve namespace)")
	addOutputFlag(cmd)
	return cmd
}

// --- wait (CLI-10.1) ---

func newWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait for a chorister resource to reach a condition",
		Long: `Blocks until a chorister resource reaches the specified condition, then exits 0.
Exits 1 if the timeout is reached before the condition is met.

Conditions: Ready, Completed, Approved, Failed
Resource types: compute, database, queue, cache, storage, sandbox, promotion`,
		Example: `  chorister wait --for Ready --type sandbox --name alice --namespace myproduct-payments-sbx-alice
  chorister wait --for Approved --type promotion --name myproduct-payments-abc123 --timeout 10m
  chorister wait --for Completed --type promotion --name myproduct-payments-abc123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			condition, _ := cmd.Flags().GetString("for")
			resourceType, _ := cmd.Flags().GetString("type")
			name, _ := cmd.Flags().GetString("name")
			namespace, _ := cmd.Flags().GetString("namespace")
			timeoutStr, _ := cmd.Flags().GetString("timeout")

			if condition == "" {
				return fmt.Errorf("--for is required (e.g. Ready, Completed, Approved)")
			}
			if resourceType == "" {
				return fmt.Errorf("--type is required (e.g. compute, database, sandbox, promotion)")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			timeout := 5 * time.Minute
			if timeoutStr != "" {
				var err error
				timeout, err = time.ParseDuration(timeoutStr)
				if err != nil {
					return fmt.Errorf("--timeout: invalid duration %q: %w", timeoutStr, err)
				}
			}

			c, err := getClient(cmd)
			if err != nil {
				return err
			}
			q := query.NewQuerier(c)

			fmt.Fprintf(cmd.OutOrStdout(), "Waiting for %s/%s condition=%q (timeout=%s)...\n", resourceType, name, condition, timeout)
			if err := q.WaitForCondition(cmd.Context(), resourceType, name, namespace, condition, timeout); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Condition %q met on %s/%s\n", condition, resourceType, name)
			return nil
		},
	}
	cmd.Flags().String("for", "", "Condition to wait for (e.g. Ready, Completed, Approved, Failed)")
	cmd.Flags().String("type", "", "Resource type (compute, database, queue, cache, storage, sandbox, promotion)")
	cmd.Flags().String("name", "", "Resource name")
	cmd.Flags().String("namespace", "", "Kubernetes namespace")
	cmd.Flags().String("timeout", "5m", "Maximum wait duration")
	return cmd
}

// --- docs ---

// newDocsCmd generates Markdown documentation for all commands.
// It is hidden from the help output but available as `chorister docs`.
func newDocsCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "docs",
		Short:  "Generate Markdown reference documentation",
		Long:   `Writes one Markdown file per command to the specified output directory. Suitable for publishing to a docs site or checking into a repository.`,
		Hidden: true,
		Example: `  chorister docs
  chorister docs --output-dir ./docs/cli`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputDir, _ := cmd.Flags().GetString("output-dir")
			if outputDir == "" {
				outputDir = "./docs/cli"
			}
			if err := os.MkdirAll(outputDir, 0750); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}
			if err := doc.GenMarkdownTree(root, outputDir); err != nil {
				return fmt.Errorf("generate docs: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Documentation written to %s\n", outputDir)
			return nil
		},
	}
	cmd.Flags().String("output-dir", "./docs/cli", "Directory to write Markdown files")
	return cmd
}
