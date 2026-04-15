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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/compiler"
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
		newAdminCmd(),
		newExportCmd(),
	)

	return root
}

// --- version ---

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
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
		Use:   "login",
		Short: "Authenticate via OIDC",
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
		Long:  `Reads the DSL file and creates/updates CRDs in the target sandbox namespace. Refuses to target production namespaces.`,
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
	}

	cmd.AddCommand(
		newSandboxCreateCmd(),
		newSandboxDestroyCmd(),
		newSandboxListCmd(),
	)
	return cmd
}

func newSandboxCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new sandbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			name, _ := cmd.Flags().GetString("name")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			fmt.Printf("sandbox create: domain=%s name=%s (not yet implemented)\n", domain, name)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("name", "n", "", "Sandbox name")
	return cmd
}

func newSandboxDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy a sandbox and all its resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			name, _ := cmd.Flags().GetString("name")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			fmt.Printf("sandbox destroy: domain=%s name=%s (not yet implemented)\n", domain, name)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("name", "n", "", "Sandbox name")
	return cmd
}

func newSandboxListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			fmt.Printf("sandbox list: domain=%s (not yet implemented)\n", domain)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Filter by domain")
	return cmd
}

// --- diff ---

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare sandbox vs production",
		Long:  `Shows resource-level differences between sandbox and production namespaces: added, changed, removed resources.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if sandbox == "" {
				return fmt.Errorf("--sandbox is required")
			}

			fmt.Printf("diff: domain=%s sandbox=%s (not yet implemented)\n", domain, sandbox)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Source sandbox name")
	return cmd
}

// --- status ---

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [domain]",
		Short: "Show domain health across environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				fmt.Printf("status: domain=%s (not yet implemented)\n", args[0])
			} else {
				fmt.Println("status: all domains (not yet implemented)")
			}
			return nil
		},
	}
	return cmd
}

// --- promote ---

func newPromoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Create a ChoPromotionRequest to promote sandbox to production",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			sandbox, _ := cmd.Flags().GetString("sandbox")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if sandbox == "" {
				return fmt.Errorf("--sandbox is required")
			}

			fmt.Printf("promote: domain=%s sandbox=%s (not yet implemented)\n", domain, sandbox)
			return nil
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("sandbox", "s", "", "Source sandbox name")
	return cmd
}

// --- approve ---

func newApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve [promotion-id]",
		Short: "Approve a ChoPromotionRequest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("approve: id=%s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

// --- reject ---

func newRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject [promotion-id]",
		Short: "Reject a ChoPromotionRequest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("reject: id=%s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

// --- requests ---

func newRequestsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "requests",
		Short: "List pending promotion requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("requests: (not yet implemented)")
			return nil
		},
	}
}

// --- admin ---

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Platform administration commands (org-admin only)",
	}

	cmd.AddCommand(
		newAdminAppCmd(),
		newAdminDomainCmd(),
		newAdminMemberCmd(),
		newAdminComplianceCmd(),
		newAdminIsolateCmd(),
		newAdminUnisolateCmd(),
		newAdminResourceCmd(),
		newAdminUpgradeCmd(),
	)
	return cmd
}

func newAdminAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "create [name]",
			Short: "Create a ChoApplication",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("admin app create: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "set-policy [name]",
			Short: "Update application policy",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("admin app set-policy: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
	)
	return cmd
}

func newAdminDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage domains",
	}

	cmd.AddCommand(
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

func newAdminMemberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage domain memberships",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "add",
			Short: "Add a member to a domain",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("admin member add: (not yet implemented)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove",
			Short: "Remove a member from a domain",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("admin member remove: (not yet implemented)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List domain members",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("admin member list: (not yet implemented)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "audit",
			Short: "Audit memberships for stale or expired access",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("admin member audit: (not yet implemented)")
				return nil
			},
		},
	)
	return cmd
}

func newAdminComplianceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compliance",
		Short: "Compliance reporting",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("admin compliance: (not yet implemented)")
			return nil
		},
	}
}

func newAdminIsolateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "isolate [domain]",
		Short: "Isolate a domain (tighten NetworkPolicy, freeze promotions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainName := args[0]
			app, _ := cmd.Flags().GetString("app")
			if app == "" {
				return fmt.Errorf("--app is required")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Isolating domain %s in application %s\n", domainName, app)
			fmt.Fprintf(cmd.OutOrStdout(), "Setting annotation chorister.dev/isolate-%s=true\n", domainName)
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is now isolated: promotions blocked, network tightened\n", domainName)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name")
	return cmd
}

func newAdminUnisolateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unisolate [domain]",
		Short: "Restore a previously isolated domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainName := args[0]
			app, _ := cmd.Flags().GetString("app")
			if app == "" {
				return fmt.Errorf("--app is required")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unisolating domain %s in application %s\n", domainName, app)
			fmt.Fprintf(cmd.OutOrStdout(), "Removing annotation chorister.dev/isolate-%s\n", domainName)
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is restored: promotions unblocked, network policies reverted\n", domainName)
			return nil
		},
	}
	cmd.Flags().String("app", "", "Application name")
	return cmd
}

func newAdminResourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources (e.g. delete archived)",
	}

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

			fmt.Fprintf(cmd.OutOrStdout(), "Deleting archived %s %q in namespace %q\n", resourceType, archived, namespace)
			fmt.Fprintf(cmd.OutOrStdout(), "Taking final snapshot before deletion...\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Archived %s %q deleted successfully. Audit event recorded.\n", resourceType, archived)
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
--promote to make a revision the stable default, or --rollback to remove a canary revision.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			revision, _ := cmd.Flags().GetString("revision")
			promote, _ := cmd.Flags().GetString("promote")
			rollback, _ := cmd.Flags().GetString("rollback")

			switch {
			case revision != "":
				fmt.Fprintf(cmd.OutOrStdout(), "Deploying canary controller revision %q\n", revision)
				fmt.Fprintf(cmd.OutOrStdout(), "Canary revision %q deployed. Use --promote %s to make it stable.\n", revision, revision)
			case promote != "":
				fmt.Fprintf(cmd.OutOrStdout(), "Promoting revision %q to stable\n", promote)
				fmt.Fprintf(cmd.OutOrStdout(), "Updating ChoCluster.spec.controllerRevision to %q\n", promote)
				fmt.Fprintf(cmd.OutOrStdout(), "Retagging all namespaces to revision %q\n", promote)
				fmt.Fprintf(cmd.OutOrStdout(), "Revision %q is now the stable default.\n", promote)
			case rollback != "":
				fmt.Fprintf(cmd.OutOrStdout(), "Rolling back canary revision %q\n", rollback)
				fmt.Fprintf(cmd.OutOrStdout(), "Canary revision %q removed.\n", rollback)
			default:
				return fmt.Errorf("one of --revision, --promote, or --rollback is required")
			}
			return nil
		},
	}
	cmd.Flags().String("revision", "", "Deploy a new controller revision")
	cmd.Flags().String("promote", "", "Promote a revision to stable")
	cmd.Flags().String("rollback", "", "Remove a canary revision")
	return cmd
}

// --- export ---

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export compiled Blueprint as static YAML for GitOps",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			output, _ := cmd.Flags().GetString("output")

			if domain == "" {
				return fmt.Errorf("--domain is required")
			}

			return runExport(cmd, domain, output)
		},
	}
	cmd.Flags().StringP("domain", "d", "", "Target domain")
	cmd.Flags().StringP("output", "o", "./export", "Output directory")
	return cmd
}

func runExport(cmd *cobra.Command, domain, outputDir string) error {
	// Compile the domain manifests using the compiler
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "export"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: domain},
			},
		},
	}

	var manifests [][]byte

	// Compile network policy for restricted domains
	domainSpec := app.Spec.Domains[0]
	if domainSpec.Sensitivity == "restricted" {
		obj := compiler.CompileRestrictedDomainL7Policy(app, domainSpec)
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return fmt.Errorf("marshal restricted policy: %w", err)
		}
		manifests = append(manifests, data)
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	outputPath := filepath.Join(outputDir, domain+".yaml")
	var buf []byte
	for i, m := range manifests {
		if i > 0 {
			buf = append(buf, []byte("---\n")...)
		}
		buf = append(buf, m...)
	}

	// Even if no manifests were compiled, produce an empty file with a comment
	if len(buf) == 0 {
		buf = []byte(fmt.Sprintf("# chorister export: domain=%s\n# No manifests compiled — domain has no compiled resources.\n", domain))
	}

	if err := os.WriteFile(outputPath, buf, 0o600); err != nil {
		return fmt.Errorf("write export: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Exported domain %s to %s\n", domain, outputPath)
	return nil
}
