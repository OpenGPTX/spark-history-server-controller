/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	reconcilehelper "github.com/kubeflow/kubeflow/components/common/reconcilehelper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubricksv1 "kubricks.io/sparkhistoryserver/api/v1"
)

const DefaultServingPort = 18080
const DefaultServingPortName = "historyport"
const AnnotationRewriteURI = "notebooks.kubeflow.org/http-rewrite-uri"
const AnnotationHeadersRequestSet = "notebooks.kubeflow.org/http-headers-request-set"

// SparkHistoryServerReconciler reconciles a SparkHistoryServer object
type SparkHistoryServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=api.core.v1,resources=deployment,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.core.v1,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.istio.io",resources=virtualservices,verbs="*"
//+kubebuilder:rbac:groups="networking.istio.io",resources=envoyfilters,verbs="*"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SparkHistoryServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *SparkHistoryServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO(user): your logic here

	// If any changes occur then reconcile function will be called.
	// Get the sparkHistoryServer object on which reconcile is called
	var sparkHistoryServer kubricksv1.SparkHistoryServer
	if err := r.Get(ctx, req.NamespacedName, &sparkHistoryServer); err != nil {
		log.Info("Unable to fetch SparkHistoryServer", "Error", err)

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Reconcile Deployment.
	if err := r.reconcileDeployment(ctx, req, &sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile Service.
	if err := r.reconcileService(ctx, req, &sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile VirtualService.
	if err := r.reconcileVirtualService(&sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile EnvoyFilter.
	if err := r.reconcileEnvoyFilter(req, &sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SparkHistoryServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubricksv1.SparkHistoryServer{}).
		Complete(r)
}

// Generates VirtualService from CR SparkHistoryServer
func generateVirtualService(instance *kubricksv1.SparkHistoryServer) (*unstructured.Unstructured, error) {
	name := instance.Name
	namespace := instance.Namespace
	clusterDomain := "cluster.local"
	prefix := fmt.Sprintf("/sparkhistory/%s", namespace)
	rewrite := "/"

	if clusterDomainFromEnv, ok := os.LookupEnv("CLUSTER_DOMAIN"); ok {
		clusterDomain = clusterDomainFromEnv
	}
	service := fmt.Sprintf("%s.%s.svc.%s", name, namespace, clusterDomain)

	vsvc := &unstructured.Unstructured{}
	vsvc.SetAPIVersion("networking.istio.io/v1alpha3")
	vsvc.SetKind("VirtualService")
	vsvc.SetName(name)
	vsvc.SetNamespace(namespace)
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{"*"}, "spec", "hosts"); err != nil {
		return nil, fmt.Errorf("Set .spec.hosts error: %v", err)
	}

	// The gateway section of the istio VirtualService spec
	istioGateway := os.Getenv("ISTIO_GATEWAY")
	if len(istioGateway) == 0 {
		istioGateway = "kubeflow/kubeflow-gateway"
	}
	// Add gateway section to istio VirtualService spec
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{istioGateway},
		"spec", "gateways"); err != nil {
		return nil, fmt.Errorf("Set .spec.gateways error: %v", err)
	}

	// The http section of the istio VirtualService spec
	http := []interface{}{
		map[string]interface{}{
			"match": []interface{}{
				map[string]interface{}{
					"uri": map[string]interface{}{
						"prefix": prefix + "/",
					},
				},
				map[string]interface{}{
					"uri": map[string]interface{}{
						"prefix": prefix,
					},
				},
			},
			"rewrite": map[string]interface{}{
				"uri": rewrite,
			},
			"route": []interface{}{
				map[string]interface{}{
					"destination": map[string]interface{}{
						"host": service,
						"port": map[string]interface{}{
							"number": int64(DefaultServingPort),
						},
					},
				},
			},
		},
	}
	// Add http section to istio VirtualService spec
	if err := unstructured.SetNestedSlice(vsvc.Object, http, "spec", "http"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

	return vsvc, nil
}

func (r *SparkHistoryServerReconciler) reconcileVirtualService(instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating VirtualService")
	virtualService, err := generateVirtualService(instance)
	if err := ctrl.SetControllerReference(instance, virtualService, r.Scheme); err != nil {
		return err
	}
	// Check if the VirtualService already exists.
	foundVirtual := &unstructured.Unstructured{}
	justCreated := false
	foundVirtual.SetAPIVersion("networking.istio.io/v1alpha3")
	foundVirtual.SetKind("VirtualService")
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundVirtual)
	if err != nil && apierrs.IsNotFound(err) {
		justCreated = true
		if err = r.Create(context.TODO(), virtualService); err != nil {
			return err
		}
		log.Info("VirtualService created")
	} else if err != nil {
		log.Info("Failed to get VirtualService")
		return err
	}

	if !justCreated && reconcilehelper.CopyVirtualService(virtualService, foundVirtual) {
		if err = r.Update(context.TODO(), foundVirtual); err != nil {
			return err
		}
		log.Info("EnvoyFilter updated")
	}

	return nil
}

// Generates EnvoyFilter from CR SparkHistoryServer
func generateEnvoyFilter(req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) (*unstructured.Unstructured, error) {
	name := instance.Name
	namespace := instance.Namespace

	vsvc := &unstructured.Unstructured{}
	vsvc.SetAPIVersion("networking.istio.io/v1alpha3")
	vsvc.SetKind("EnvoyFilter")
	vsvc.SetName(name)
	vsvc.SetNamespace(namespace)
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{"*"}, "spec", "hosts"); err != nil {
		return nil, fmt.Errorf("Set .spec.hosts error: %v", err)
	}

	// The workloadSelector section of the istio EnvoyFilter spec
	workloadSelector := map[string]interface{}{
		"labels": map[string]interface{}{
			"app.kubernetes.io/name":     req.Name,
			"app.kubernetes.io/instance": req.Namespace,
		},
	}
	// Add workloadSelector section to istio EnvoyFilter spec
	if err := unstructured.SetNestedField(vsvc.Object, workloadSelector, "spec", "workloadSelector"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

	// The configPatches section of the istio EnvoyFilter spec
	var inlineCode = "function envoy_on_response(response_handle, context)\n    response_handle:headers():replace(\"location\", \"\");\nend\n"
	configPatches := []interface{}{
		map[string]interface{}{
			"applyTo": "HTTP_FILTER",
			"match": map[string]interface{}{
				"context": "SIDECAR_INBOUND",
				"listener": map[string]interface{}{
					"filterChain": map[string]interface{}{
						"filter": map[string]interface{}{
							"name": "envoy.filters.network.http_connection_manager",
							"subFilter": map[string]interface{}{
								"name": "envoy.filters.http.router",
							},
						},
					},
				},
			},
			"patch": map[string]interface{}{
				"operation": "INSERT_BEFORE",
				"value": map[string]interface{}{
					"name": "envoy.lua",
					"typed_config": map[string]interface{}{
						"@type":      "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua",
						"inlineCode": string(inlineCode),
					},
				},
			},
		},
	}
	// Add configPatches section to istio EnvoyFilter spec
	if err := unstructured.SetNestedSlice(vsvc.Object, configPatches, "spec", "configPatches"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

	return vsvc, nil

}

func (r *SparkHistoryServerReconciler) reconcileEnvoyFilter(req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating EnvoyFilter")
	envoyFilter, err := generateEnvoyFilter(req, instance, log)
	if err := ctrl.SetControllerReference(instance, envoyFilter, r.Scheme); err != nil {
		return err
	}
	// Check if the EnvoyFilter already exists.
	foundEnvoy := &unstructured.Unstructured{}
	justCreated := false
	foundEnvoy.SetAPIVersion("networking.istio.io/v1alpha3")
	foundEnvoy.SetKind("EnvoyFilter")
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundEnvoy)
	if err != nil && apierrs.IsNotFound(err) {
		justCreated = true
		if err = r.Create(context.TODO(), envoyFilter); err != nil {
			return err
		}
		log.Info("EnvoyFilter created")
	} else if err != nil {
		log.Info("Failed to get EnvoyFilter")
		return err
	}

	if !justCreated && CopyEnvoyFilter(envoyFilter, foundEnvoy) {
		if err = r.Update(context.TODO(), foundEnvoy); err != nil {
			return err
		}
		log.Info("EnvoyFilter updated")
	}

	return nil
}

//https://github.com/kubeflow/kubeflow/blob/7f4231de77ea/components/common/reconcilehelper/util.go#L199
// Copy configuration related fields to another instance and returns true if there
// is a diff and thus needs to update.
func CopyEnvoyFilter(from, to *unstructured.Unstructured) bool {
	fromSpec, found, err := unstructured.NestedMap(from.Object, "spec")
	if !found {
		return false
	}
	if err != nil {
		return false
	}

	toSpec, found, err := unstructured.NestedMap(to.Object, "spec")
	if !found || err != nil {
		unstructured.SetNestedMap(to.Object, fromSpec, "spec")
		return true
	}

	requiresUpdate := !reflect.DeepEqual(fromSpec, toSpec)
	if requiresUpdate {
		unstructured.SetNestedMap(to.Object, fromSpec, "spec")
	}
	return requiresUpdate
}

// Generates Service from CR SparkHistoryServer
func generateService(instance *kubricksv1.SparkHistoryServer) (*corev1.Service, error) {
	var sparkhistoryserverService *corev1.Service
	sparkhistoryserverService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name":     instance.Name,
				"app.kubernetes.io/instance": instance.Namespace,
			},
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       DefaultServingPort,
					TargetPort: intstr.FromString(DefaultServingPortName),
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":     instance.Name,
				"app.kubernetes.io/instance": instance.Namespace,
			},
		},
	}

	return sparkhistoryserverService, nil
}

func (r *SparkHistoryServerReconciler) reconcileService(ctx context.Context, req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating Service")
	service, err := generateService(instance)
	if err := ctrl.SetControllerReference(instance, service, r.Scheme); err != nil {
		return err
	}

	var foundService corev1.Service
	if err = r.Get(ctx, req.NamespacedName, &foundService); err != nil {
		if apierrs.IsNotFound(err) {
			if err = r.Create(ctx, service); err != nil {
				return err
			}
			log.Info("Service created")
		} else {
			log.Info("Failed to get Service")
			return err
		}
	} else if !reflect.DeepEqual(service.Spec, foundService.Spec) {
		service.ObjectMeta = foundService.ObjectMeta
		service.Spec.ClusterIP = foundService.Spec.ClusterIP
		if err = r.Update(context.TODO(), service); err != nil {
			return err
		}
		log.Info("Service updated")
	}

	return nil
}

// Generates Deployment from CR SparkHistoryServer
func generateDeployment(instance *kubricksv1.SparkHistoryServer) (*appsv1.Deployment, error) {

	var historyCommand = "export SPARK_HISTORY_OPTS=\"$SPARK_HISTORY_OPTS \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.WebIdentityTokenCredentialsProvider \\\n"
	historyCommand += "  -Dspark.history.fs.logDirectory=s3a://" + instance.Spec.Bucket + "/pipelines/" + instance.Namespace + "/history \\\n"
	historyCommand += "  -Dspark.ui.proxyBase=/sparkhistory/" + instance.Namespace + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.enabled=" + strconv.FormatBool(instance.Spec.Cleaner.Enabled) + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.maxAge=" + instance.Spec.Cleaner.MaxAge + "\";\n"
	historyCommand += "/opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;\n"

	var sparkhistoryserverDeployment *appsv1.Deployment
	sparkhistoryserverDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name":     instance.Name,
				"app.kubernetes.io/instance": instance.Namespace,
			},
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: instance.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     instance.Name,
					"app.kubernetes.io/instance": instance.Namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     instance.Name,
						"app.kubernetes.io/instance": instance.Namespace,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.Spec.ServiceAccountName,
					Containers: []corev1.Container{
						corev1.Container{
							Name: instance.Name,
							Env: []corev1.EnvVar{
								{
									Name:  "SPARK_NO_DAEMONIZE",
									Value: "false",
								},
							},
							Image: instance.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: DefaultServingPort,
									Name:          DefaultServingPortName,
								},
							},
							ImagePullPolicy: instance.Spec.ImagePullPolicy,
							Resources:       *instance.Spec.Resources,
							Command: []string{
								"/bin/sh",
								"-c",
								string(historyCommand),
							},
						},
					},
				},
			},
		},
	}

	return sparkhistoryserverDeployment, nil
}

func (r *SparkHistoryServerReconciler) reconcileDeployment(ctx context.Context, req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating Deployment")
	deployment, err := generateDeployment(instance)
	if err := ctrl.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return err
	}

	var foundDeployment appsv1.Deployment
	if err = r.Get(ctx, req.NamespacedName, &foundDeployment); err != nil {
		if apierrs.IsNotFound(err) {
			if err = r.Create(ctx, deployment); err != nil {
				return err
			}
			log.Info("Deployment created")
		} else {
			log.Info("Failed to get Deployment")
			return err
		}
	} else if !reflect.DeepEqual(deployment.Spec, foundDeployment.Spec) {
		deployment.ObjectMeta = foundDeployment.ObjectMeta
		if err = r.Update(context.TODO(), deployment); err != nil {
			return err
		}
		log.Info("Deployment updated")
	}

	return nil
}
