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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/audit"
	"github.com/chorister-dev/chorister/internal/compiler"
)

const (
	clusterLabelManagedBy = "chorister.dev/managed-by"
	clusterLabelComponent = "chorister.dev/component"
)

// ChoClusterReconciler reconciles a ChoCluster object.
type ChoClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	AuditLogger audit.Logger
}

// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoCluster spec.
func (r *ChoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cluster := &choristerv1alpha1.ChoCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Audit: log the reconciliation start (fail-fast if audit sink fails)
	if r.AuditLogger == nil {
		log.Error(nil, "AuditLogger not configured, blocking reconciliation")
		return ctrl.Result{}, fmt.Errorf("audit write failed, blocking reconciliation: AuditLogger not configured")
	}
	if err := r.AuditLogger.Log(ctx, audit.Event{
		Timestamp: time.Now(),
		Action:    "Reconcile",
		Resource:  "ChoCluster/" + cluster.Name,
		Result:    "started",
	}); err != nil {
		log.Error(err, "Audit write failed, blocking reconciliation")
		setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "AuditReady",
			Status:  metav1.ConditionFalse,
			Reason:  "AuditWriteFailed",
			Message: fmt.Sprintf("Audit sink write failed: %v", err),
		})
		_ = r.Status().Update(ctx, cluster)
		return ctrl.Result{}, fmt.Errorf("audit write failed, blocking reconciliation: %w", err)
	}

	// Initialize status maps
	if cluster.Status.OperatorStatus == nil {
		cluster.Status.OperatorStatus = make(map[string]string)
	}

	// Phase 21.1: Ensure default sizing templates are installed (before status accumulates)
	if err := r.reconcileDefaultSizingTemplates(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Phase 12.1: Reconcile operators
	if cluster.Spec.Operators != nil {
		if err := r.reconcileOperators(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Phase P3.4: Reconcile cloud provider plugin for object storage
	if cluster.Spec.CloudProvider != nil {
		if err := r.reconcileCloudProvider(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Phase 11.1: Reconcile observability stack
	if err := r.reconcileObservability(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Phase 12.3: Validate StorageClass for encryption
	r.validateStorageClass(ctx, cluster)

	// Phase 11.3: Reconcile Grafana dashboards
	if err := r.reconcileGrafanaDashboards(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileKubeBench(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Phase 16.1: Reconcile cert-manager ClusterIssuer
	if err := r.reconcileCertManager(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	cluster.Status.Phase = "Ready"
	setCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "ChoCluster reconciliation completed",
	})
	if err := r.Status().Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation completed", "cluster", cluster.Name)
	return ctrl.Result{}, nil
}

func (r *ChoClusterReconciler) reconcileKubeBench(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	if err := r.ensureClusterNamespace(ctx, "cho-system", map[string]string{clusterLabelManagedBy: "chocluster", clusterLabelComponent: "security"}); err != nil {
		return err
	}

	cronJob := &batchv1.CronJob{}
	key := types.NamespacedName{Name: "kube-bench", Namespace: "cho-system"}
	err := r.Get(ctx, key, cronJob)
	if errors.IsNotFound(err) {
		cronJob = &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
				Labels: map[string]string{
					clusterLabelManagedBy: "chocluster",
					clusterLabelComponent: "kube-bench",
				},
			},
			Spec: batchv1.CronJobSpec{
				Schedule: "0 4 * * 0",
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Containers: []corev1.Container{{
									Name:  "kube-bench",
									Image: "docker.io/aquasec/kube-bench:latest",
									Args:  []string{"run", "--targets=node,master"},
								}},
							},
						},
					},
				},
			},
		}
		if err := r.Create(ctx, cronJob); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	cluster.Status.CISBenchmark = "kube-bench scheduled weekly"
	return nil
}

// ---------------------------------------------------------------------------
// Phase 12.1: Operator lifecycle
// ---------------------------------------------------------------------------

type operatorDef struct {
	name      string
	namespace string
	image     string
}

func operatorDefs(ops *choristerv1alpha1.OperatorVersions) []operatorDef {
	var defs []operatorDef
	if ops.Kro != "" {
		defs = append(defs, operatorDef{"kro", "cho-kro-system", "ghcr.io/awslabs/kro:" + ops.Kro})
	}
	if ops.StackGres != "" {
		defs = append(defs, operatorDef{"stackgres", "cho-stackgres-system", "stackgres/operator:" + ops.StackGres})
	}
	if ops.NATS != "" {
		defs = append(defs, operatorDef{"nats", "cho-nats-system", "natsio/nats-server:" + ops.NATS})
	}
	if ops.Dragonfly != "" {
		defs = append(defs, operatorDef{"dragonfly", "cho-dragonfly-system", "docker.dragonflydb.io/dragonflydb/operator:" + ops.Dragonfly})
	}
	if ops.CertManager != "" {
		defs = append(defs, operatorDef{"cert-manager", "cho-cert-manager-system", "quay.io/jetstack/cert-manager-controller:" + ops.CertManager})
	}
	if ops.Gatekeeper != "" {
		defs = append(defs, operatorDef{"gatekeeper", "cho-gatekeeper-system", "openpolicyagent/gatekeeper:" + ops.Gatekeeper})
	}
	if ops.Tetragon != "" {
		defs = append(defs, operatorDef{"tetragon", "cho-tetragon-system", "quay.io/cilium/tetragon:" + ops.Tetragon})
	}
	return defs
}

func (r *ChoClusterReconciler) reconcileOperators(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	log := logf.FromContext(ctx)

	for _, def := range operatorDefs(cluster.Spec.Operators) {
		if err := r.ensureClusterNamespace(ctx, def.namespace, map[string]string{
			clusterLabelManagedBy: "chocluster",
			clusterLabelComponent: "operator-" + def.name,
		}); err != nil {
			return err
		}

		if err := r.ensureOperatorDeployment(ctx, def); err != nil {
			return err
		}

		cluster.Status.OperatorStatus[def.name] = "Installed"
		log.Info("Operator reconciled", "operator", def.name)
	}

	return nil
}

// defaultCloudProviderImages maps provider to default controller image.
var defaultCloudProviderImages = map[string]string{
	"aws":   "public.ecr.aws/aws-controllers-k8s/s3-controller:v1.0.17",
	"gcp":   "gcr.io/config-connector/operator:latest",
	"azure": "mcr.microsoft.com/aks/aso/controller:v2.10.0",
}

// reconcileCloudProvider deploys the cloud provider controller for object storage.
func (r *ChoClusterReconciler) reconcileCloudProvider(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	log := logf.FromContext(ctx)
	cp := cluster.Spec.CloudProvider

	image := cp.Image
	if image == "" {
		var ok bool
		image, ok = defaultCloudProviderImages[cp.Provider]
		if !ok {
			return fmt.Errorf("unsupported cloud provider: %s", cp.Provider)
		}
	}

	nsName := "cho-cloud-provider-system"
	if err := r.ensureClusterNamespace(ctx, nsName, map[string]string{
		clusterLabelManagedBy: "chocluster",
		clusterLabelComponent: "cloud-provider-" + cp.Provider,
	}); err != nil {
		return err
	}

	def := operatorDef{
		name:      "cloud-provider-" + cp.Provider,
		namespace: nsName,
		image:     image,
	}
	if err := r.ensureOperatorDeployment(ctx, def); err != nil {
		return err
	}

	cluster.Status.OperatorStatus["cloud-provider"] = cp.Provider
	log.Info("Cloud provider reconciled", "provider", cp.Provider)
	return nil
}

func (r *ChoClusterReconciler) ensureOperatorDeployment(ctx context.Context, def operatorDef) error {
	deployName := def.name + "-operator"
	replicas := int32(1)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: def.namespace}, deploy)
	if errors.IsNotFound(err) {
		deploy = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: def.namespace,
				Labels: map[string]string{
					clusterLabelManagedBy: "chocluster",
					clusterLabelComponent: "operator-" + def.name,
					"app":                 def.name,
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": def.name},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": def.name},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  def.name,
								Image: def.image,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							},
						},
					},
				},
			},
		}
		return r.Create(ctx, deploy)
	}
	return err
}

// ---------------------------------------------------------------------------
// Phase 11.1: Observability stack (Grafana LGTM)
// ---------------------------------------------------------------------------

type resolvedVersions struct {
	Alloy, Mimir, Loki, Tempo, Grafana string
}

func defaultVersions() resolvedVersions {
	return resolvedVersions{
		Alloy:   "latest",
		Mimir:   "latest",
		Loki:    "latest",
		Tempo:   "latest",
		Grafana: "latest",
	}
}

func monitoringNamespace(cluster *choristerv1alpha1.ChoCluster) string {
	if cluster.Spec.Observability != nil && cluster.Spec.Observability.MonitoringNamespace != "" {
		return cluster.Spec.Observability.MonitoringNamespace
	}
	return "cho-monitoring"
}

func (r *ChoClusterReconciler) reconcileObservability(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	// Skip if explicitly disabled
	if cluster.Spec.Observability != nil && cluster.Spec.Observability.Enabled != nil && !*cluster.Spec.Observability.Enabled {
		cluster.Status.ObservabilityReady = false
		return nil
	}

	monNs := monitoringNamespace(cluster)

	if err := r.ensureClusterNamespace(ctx, monNs, map[string]string{
		clusterLabelManagedBy: "chocluster",
		clusterLabelComponent: "monitoring",
	}); err != nil {
		return err
	}

	versions := defaultVersions()
	if cluster.Spec.Observability != nil && cluster.Spec.Observability.Versions != nil {
		v := cluster.Spec.Observability.Versions
		if v.Alloy != "" {
			versions.Alloy = v.Alloy
		}
		if v.Mimir != "" {
			versions.Mimir = v.Mimir
		}
		if v.Loki != "" {
			versions.Loki = v.Loki
		}
		if v.Tempo != "" {
			versions.Tempo = v.Tempo
		}
		if v.Grafana != "" {
			versions.Grafana = v.Grafana
		}
	}

	// Resolve retention defaults
	retention := resolvedRetention{
		Metrics: "30d",
		Logs:    "14d",
		Traces:  "7d",
	}
	if cluster.Spec.Observability != nil && cluster.Spec.Observability.Retention != nil {
		ret := cluster.Spec.Observability.Retention
		if ret.Metrics != "" {
			retention.Metrics = ret.Metrics
		}
		if ret.Logs != "" {
			retention.Logs = ret.Logs
		}
		if ret.Traces != "" {
			retention.Traces = ret.Traces
		}
	}

	// Resolve object storage bucket from cloud provider for LGTM storage backend
	storageBucket := ""
	if cluster.Spec.CloudProvider != nil {
		storageBucket = fmt.Sprintf("cho-observability-%s", cluster.Name)
	}

	components := []struct {
		name  string
		image string
		port  int32
		args  []string
		env   []corev1.EnvVar
	}{
		{"loki", "grafana/loki:" + versions.Loki, 3100,
			lokiArgs(storageBucket, retention.Logs),
			lokiEnv(storageBucket),
		},
		{"mimir", "grafana/mimir:" + versions.Mimir, 9009,
			mimirArgs(storageBucket, retention.Metrics),
			mimirEnv(storageBucket),
		},
		{"tempo", "grafana/tempo:" + versions.Tempo, 3200,
			tempoArgs(storageBucket, retention.Traces),
			tempoEnv(storageBucket),
		},
		{"alloy", "grafana/alloy:" + versions.Alloy, 12345, nil, nil},
		{"grafana", "grafana/grafana:" + versions.Grafana, 3000, nil, nil},
	}

	for _, comp := range components {
		if err := r.ensureComponentDeployment(ctx, monNs, comp.name, comp.image, comp.port, comp.args, comp.env); err != nil {
			return err
		}
		if err := r.ensureComponentService(ctx, monNs, comp.name, comp.port); err != nil {
			return err
		}
	}

	cluster.Status.ObservabilityReady = true
	return nil
}

type resolvedRetention struct {
	Metrics, Logs, Traces string
}

func lokiArgs(bucket, logsRetention string) []string {
	args := []string{"-config.expand-env=true"}
	if bucket != "" {
		args = append(args,
			"-common.storage.backend=s3",
			"-common.storage.s3.bucket-names="+bucket+"-logs",
		)
	}
	args = append(args, "-limits.retention-period="+logsRetention)
	return args
}

func lokiEnv(bucket string) []corev1.EnvVar {
	if bucket == "" {
		return nil
	}
	return []corev1.EnvVar{
		{Name: "LOKI_S3_BUCKET", Value: bucket + "-logs"},
	}
}

func mimirArgs(bucket, metricsRetention string) []string {
	args := []string{"-config.expand-env=true"}
	if bucket != "" {
		args = append(args,
			"-blocks-storage.backend=s3",
			"-blocks-storage.s3.bucket-name="+bucket+"-metrics",
		)
	}
	args = append(args, "-compactor.blocks-retention-period="+metricsRetention)
	return args
}

func mimirEnv(bucket string) []corev1.EnvVar {
	if bucket == "" {
		return nil
	}
	return []corev1.EnvVar{
		{Name: "MIMIR_S3_BUCKET", Value: bucket + "-metrics"},
	}
}

func tempoArgs(bucket, tracesRetention string) []string {
	args := []string{"-config.expand-env=true"}
	if bucket != "" {
		args = append(args,
			"-storage.trace.backend=s3",
			"-storage.trace.s3.bucket="+bucket+"-traces",
		)
	}
	args = append(args, "-compactor.compaction.block-retention="+tracesRetention)
	return args
}

func tempoEnv(bucket string) []corev1.EnvVar {
	if bucket == "" {
		return nil
	}
	return []corev1.EnvVar{
		{Name: "TEMPO_S3_BUCKET", Value: bucket + "-traces"},
	}
}

func (r *ChoClusterReconciler) ensureComponentDeployment(ctx context.Context, namespace, name, image string, port int32, args []string, env []corev1.EnvVar) error {
	replicas := int32(1)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploy)
	if errors.IsNotFound(err) {
		container := corev1.Container{
			Name:  name,
			Image: image,
			Ports: []corev1.ContainerPort{
				{ContainerPort: port, Protocol: corev1.ProtocolTCP},
			},
		}
		if len(args) > 0 {
			container.Args = args
		}
		if len(env) > 0 {
			container.Env = env
		}
		deploy = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					clusterLabelManagedBy: "chocluster",
					clusterLabelComponent: name,
					"app":                 name,
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": name},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": name},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{container},
					},
				},
			},
		}
		return r.Create(ctx, deploy)
	}
	if err != nil {
		return err
	}
	// Update image if changed
	if len(deploy.Spec.Template.Spec.Containers) > 0 && deploy.Spec.Template.Spec.Containers[0].Image != image {
		deploy.Spec.Template.Spec.Containers[0].Image = image
		return r.Update(ctx, deploy)
	}
	return nil
}

func (r *ChoClusterReconciler) ensureComponentService(ctx context.Context, namespace, name string, port int32) error {
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, svc)
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					clusterLabelManagedBy: "chocluster",
					clusterLabelComponent: name,
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": name},
				Ports: []corev1.ServicePort{
					{Port: port, TargetPort: intstr.FromInt32(port), Protocol: corev1.ProtocolTCP},
				},
			},
		}
		return r.Create(ctx, svc)
	}
	return err
}

// ---------------------------------------------------------------------------
// Phase 12.3: StorageClass validation
// ---------------------------------------------------------------------------

func (r *ChoClusterReconciler) validateStorageClass(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) {
	log := logf.FromContext(ctx)

	scList := &storagev1.StorageClassList{}
	if err := r.List(ctx, scList); err != nil {
		log.Info("Could not list StorageClasses for encryption validation", "error", err)
		setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "EncryptedStorageAvailable",
			Status:  metav1.ConditionUnknown,
			Reason:  "ListFailed",
			Message: "Could not list StorageClasses",
		})
		return
	}

	for _, sc := range scList.Items {
		if isEncryptedStorageClass(sc) {
			setCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:    "EncryptedStorageAvailable",
				Status:  metav1.ConditionTrue,
				Reason:  "Found",
				Message: fmt.Sprintf("Encrypted StorageClass found: %s", sc.Name),
			})
			return
		}
	}

	log.Info("No encrypted StorageClass found; recommended for production use")
	setCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:    "EncryptedStorageAvailable",
		Status:  metav1.ConditionFalse,
		Reason:  "NotFound",
		Message: "No encrypted StorageClass found. Recommended for production use.",
	})
}

func isEncryptedStorageClass(sc storagev1.StorageClass) bool {
	if v, ok := sc.Annotations["storageclass.kubernetes.io/is-encrypted"]; ok && v == "true" {
		return true
	}
	if sc.Parameters != nil {
		if _, ok := sc.Parameters["encrypted"]; ok {
			return true
		}
		if _, ok := sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"]; ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Phase 11.3: Grafana dashboards
// ---------------------------------------------------------------------------

func (r *ChoClusterReconciler) reconcileGrafanaDashboards(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	if cluster.Spec.Observability != nil && cluster.Spec.Observability.Enabled != nil && !*cluster.Spec.Observability.Enabled {
		return nil
	}

	monNs := monitoringNamespace(cluster)

	appList := &choristerv1alpha1.ChoApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		return fmt.Errorf("list ChoApplications for dashboards: %w", err)
	}

	for i := range appList.Items {
		app := &appList.Items[i]
		for _, domain := range app.Spec.Domains {
			dashboardName := fmt.Sprintf("dashboard-%s-%s", app.Name, domain.Name)
			dashboardJSON := generateDashboardJSON(app.Name, domain.Name)

			cm := &corev1.ConfigMap{}
			err := r.Get(ctx, types.NamespacedName{Name: dashboardName, Namespace: monNs}, cm)
			if errors.IsNotFound(err) {
				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dashboardName,
						Namespace: monNs,
						Labels: map[string]string{
							clusterLabelManagedBy: "chocluster",
							"grafana_dashboard":   "1",
						},
					},
					Data: map[string]string{
						dashboardName + ".json": dashboardJSON,
					},
				}
				if err := r.Create(ctx, cm); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}

	return nil
}

func generateDashboardJSON(appName, domainName string) string {
	dashboard := map[string]any{
		"editable":      true,
		"schemaVersion": 39,
		"tags":          []string{"chorister", appName, domainName},
		"title":         fmt.Sprintf("%s / %s", appName, domainName),
		"uid":           fmt.Sprintf("cho-%s-%s", appName, domainName),
		"version":       1,
		"time":          map[string]string{"from": "now-6h", "to": "now"},
		"panels": []map[string]any{
			{
				"title":       "Pod Status",
				"type":        "stat",
				"datasource":  map[string]string{"type": "prometheus", "uid": "mimir"},
				"description": fmt.Sprintf("Pod status for %s/%s", appName, domainName),
			},
			{
				"title":       "Resource Usage",
				"type":        "timeseries",
				"datasource":  map[string]string{"type": "prometheus", "uid": "mimir"},
				"description": fmt.Sprintf("Resource usage for %s/%s", appName, domainName),
			},
			{
				"title":       "Network Flows",
				"type":        "timeseries",
				"datasource":  map[string]string{"type": "prometheus", "uid": "mimir"},
				"description": fmt.Sprintf("Network flows for %s/%s", appName, domainName),
			},
		},
	}
	b, _ := json.MarshalIndent(dashboard, "", "  ")
	return string(b)
}

// ---------------------------------------------------------------------------
// Phase 16.1: cert-manager ClusterIssuer
// ---------------------------------------------------------------------------

func (r *ChoClusterReconciler) reconcileCertManager(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	// Only set up ClusterIssuer if cert-manager operator is configured
	if cluster.Spec.Operators == nil || cluster.Spec.Operators.CertManager == "" {
		return nil
	}

	issuer := &unstructured.Unstructured{}
	issuer.SetGroupVersionKind(certManagerClusterIssuerGVK)
	issuer.SetName("chorister-cluster-issuer")
	issuer.Object["spec"] = map[string]any{
		"selfSigned": map[string]any{},
	}

	return ensureUnstructured(ctx, r.Client, issuer)
}

var certManagerClusterIssuerGVK = schema.GroupVersionKind{
	Group:   "cert-manager.io",
	Version: "v1",
	Kind:    "ClusterIssuer",
}

// ---------------------------------------------------------------------------
// Phase 21.1: Default sizing templates
// ---------------------------------------------------------------------------

func (r *ChoClusterReconciler) reconcileDefaultSizingTemplates(ctx context.Context, cluster *choristerv1alpha1.ChoCluster) error {
	if cluster.Spec.SizingTemplates != nil && len(cluster.Spec.SizingTemplates) > 0 {
		// User has already defined templates; set condition and return
		setCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "SizingTemplatesAvailable",
			Status:  metav1.ConditionTrue,
			Reason:  "UserDefined",
			Message: fmt.Sprintf("%d resource type(s) have sizing templates defined", len(cluster.Spec.SizingTemplates)),
		})
		return nil
	}

	// Install default templates
	cluster.Spec.SizingTemplates = compiler.DefaultSizingTemplates()
	if err := r.Update(ctx, cluster); err != nil {
		return fmt.Errorf("install default sizing templates: %w", err)
	}

	// Re-fetch to get the server version (Update returns server state which may
	// not include locally accumulated status changes).
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster); err != nil {
		return fmt.Errorf("re-fetch cluster after sizing template install: %w", err)
	}

	// Re-initialize status maps after re-fetch
	if cluster.Status.OperatorStatus == nil {
		cluster.Status.OperatorStatus = make(map[string]string)
	}

	setCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:    "SizingTemplatesAvailable",
		Status:  metav1.ConditionTrue,
		Reason:  "DefaultsInstalled",
		Message: "Default sizing templates installed for compute, database, cache, and queue",
	})

	return nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (r *ChoClusterReconciler) ensureClusterNamespace(ctx context.Context, name string, labels map[string]string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, ns)
	if errors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}
		return r.Create(ctx, ns)
	}
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoCluster{}).
		Named("chocluster").
		Complete(r)
}
