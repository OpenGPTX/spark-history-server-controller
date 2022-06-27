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
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

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
	// Get the sparkhistoryserver object on which reconcile is called
	var sparkhistoryserver kubricksv1.SparkHistoryServer
	if err := r.Get(ctx, req.NamespacedName, &sparkhistoryserver); err != nil {
		//log.Error(err, "unable to fetch Sparkhistoryserver")
		log.Info("Unable to fetch Sparkhistoryserver", "Error", err)

		// Delete Deployment if it exists
		var websitesDeployment appsv1.Deployment
		if err := r.Get(ctx, req.NamespacedName, &websitesDeployment); err == nil {
			return r.RemoveDeployment(ctx, &websitesDeployment, log)
		}

		// // Delete service if it exists
		// var sparkhistoryserverService corev1.Service
		// if err := r.Get(ctx, req.NamespacedName, &sparkhistoryserverService); err == nil {
		// 	return r.RemoveService(ctx, &sparkhistoryserverService, log)
		// }

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If we have the sparkhistoryserver resource we need to ensure that the child resources are created as well.
	log.Info("Ensuring Deployment is created", "sparkhistoryserver", req.NamespacedName)
	var sparkhistoryserverDeployment appsv1.Deployment
	if err := r.Get(ctx, req.NamespacedName, &sparkhistoryserverDeployment); err != nil {
		log.Info("unable to fetch Deployment for sparkhistoryserver", "sparkhistoryserver", req.NamespacedName)
		// Create a deployment
		return r.CreateDeployment(ctx, req, sparkhistoryserver, log)
	}

	// Reconcile Service.
	err := r.reconcileService(ctx, req, &sparkhistoryserver, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile VirtualService.
	err = r.reconcileVirtualService(&sparkhistoryserver, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile EnvoyFilter.
	err = r.reconcileEnvoyFilter(req, &sparkhistoryserver, log)
	if err != nil {
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

// CreateDeployment creates the deployment in the cluster.
func (r *SparkHistoryServerReconciler) CreateDeployment(ctx context.Context, req ctrl.Request, sparkhistoryserver kubricksv1.SparkHistoryServer, log logr.Logger) (ctrl.Result, error) {

	var historyCommand = "export SPARK_HISTORY_OPTS=\"$SPARK_HISTORY_OPTS \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.WebIdentityTokenCredentialsProvider \\\n"
	historyCommand += "  -Dspark.history.fs.logDirectory=s3a://" + sparkhistoryserver.Spec.Bucket + "/pipelines/" + req.Namespace + "/history \\\n"
	historyCommand += "  -Dspark.ui.proxyBase=/sparkhistory/" + req.Namespace + " \\\n"
	//	historyCommand += "  -Dspark.ui.reverseProxy=true \\\n"
	//	historyCommand += "  -Dspark.ui.reverseProxyUrl=https://kubeflow.at.onplural.sh/sparkhistory/" + req.Namespace + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.enabled=" + strconv.FormatBool(sparkhistoryserver.Spec.Cleaner.Enabled) + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.maxAge=" + sparkhistoryserver.Spec.Cleaner.MaxAge + "\";\n"
	historyCommand += "/opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;\n"

	var sparkhistoryserverDeployment *appsv1.Deployment
	sparkhistoryserverDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name":     req.Name,
				"app.kubernetes.io/instance": req.Namespace,
			},
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: sparkhistoryserver.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     req.Name,
					"app.kubernetes.io/instance": req.Namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     req.Name,
						"app.kubernetes.io/instance": req.Namespace,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sparkhistoryserver.Spec.ServiceAccountName,
					Containers: []corev1.Container{
						corev1.Container{
							Name: sparkhistoryserver.Name,
							Env: []corev1.EnvVar{
								{
									Name:  "SPARK_NO_DAEMONIZE",
									Value: "false",
								},
							},
							Image: sparkhistoryserver.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 18080,
									Name:          "historyport",
								},
							},
							ImagePullPolicy: sparkhistoryserver.Spec.ImagePullPolicy,
							Resources:       *sparkhistoryserver.Spec.Resources,
							// Resources: corev1.ResourceRequirements{
							// 	Requests: corev1.ResourceList{
							// 		corev1.ResourceCPU: resource.MustParse("1000m"), //sparkhistoryserver.Spec.CPURequest, // https://github.com/GoogleCloudPlatform/flink-on-k8s-operator/blob/master/controllers/flinkcluster_converter_test.go
							// 	},
							// },
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
	if err := r.Create(ctx, sparkhistoryserverDeployment); err != nil {
		log.Error(err, "unable to create website deployment for Website", "website", sparkhistoryserverDeployment)
		return ctrl.Result{}, err
	}

	log.V(1).Info("created website deployment for Website run", "websitePod", sparkhistoryserverDeployment)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// RemoveDeployment deletes deployment from the cluster
func (r *SparkHistoryServerReconciler) RemoveDeployment(ctx context.Context, deplmtToRemove *appsv1.Deployment, log logr.Logger) (ctrl.Result, error) {
	name := deplmtToRemove.Name
	if err := r.Delete(ctx, deplmtToRemove); err != nil {
		log.Error(err, "unable to delete website deployment for Website", "website", deplmtToRemove.Name)
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed website deployment for Website run", "websitePod", name)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// CreateService creates the desired service in the cluster
func (r *SparkHistoryServerReconciler) CreateService(ctx context.Context, req ctrl.Request, sparkhistoryserver kubricksv1.SparkHistoryServer, log logr.Logger) (ctrl.Result, error) {
	var sparkhistoryserverService *corev1.Service
	sparkhistoryserverService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name":     req.Name,
				"app.kubernetes.io/instance": req.Namespace,
			},
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       18080,
					TargetPort: intstr.FromString("historyport"),
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":     req.Name,
				"app.kubernetes.io/instance": req.Namespace,
			},
		},
	}
	if err := r.Create(ctx, sparkhistoryserverService); err != nil {
		log.Error(err, "unable to create sparkhistoryserver service for sparkhistoryserver", "sparkhistoryserver", sparkhistoryserverService)
		return ctrl.Result{}, err
	}

	log.V(1).Info("created sparkhistoryserver service for sparkhistoryserver run", "sparkhistoryserverPod", sparkhistoryserverService)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// RemoveService deletes the service from the cluster.
func (r *SparkHistoryServerReconciler) RemoveService(ctx context.Context, serviceToRemove *corev1.Service, log logr.Logger) (ctrl.Result, error) {
	name := serviceToRemove.Name
	if err := r.Delete(ctx, serviceToRemove); err != nil {
		log.Error(err, "unable to delete sparkhistoryserver service for sparkhistoryserver", "sparkhistoryserver", serviceToRemove.Name)
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed sparkhistoryserver service for sparkhistoryserver run", "sparkhistoryserverPod", name)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func virtualServiceName(kfName string, namespace string) string {
	return fmt.Sprintf("notebook-%s-%s", namespace, kfName)
}

func generateVirtualService(instance *kubricksv1.SparkHistoryServer) (*unstructured.Unstructured, error) {
	name := instance.Name
	namespace := instance.Namespace
	clusterDomain := "cluster.local"
	prefix := fmt.Sprintf("/sparkhistory/%s", namespace)

	// unpack annotations from Notebook resource
	annotations := make(map[string]string)
	for k, v := range instance.ObjectMeta.Annotations {
		annotations[k] = v
	}

	rewrite := fmt.Sprintf("/")
	// If AnnotationRewriteURI is present, use this value for "rewrite"
	if _, ok := annotations[AnnotationRewriteURI]; ok && len(annotations[AnnotationRewriteURI]) > 0 {
		rewrite = annotations[AnnotationRewriteURI]
	}

	if clusterDomainFromEnv, ok := os.LookupEnv("CLUSTER_DOMAIN"); ok {
		clusterDomain = clusterDomainFromEnv
	}
	service := fmt.Sprintf("%s.%s.svc.%s", name, namespace, clusterDomain)

	vsvc := &unstructured.Unstructured{}
	vsvc.SetAPIVersion("networking.istio.io/v1alpha3")
	vsvc.SetKind("VirtualService")
	vsvc.SetName(virtualServiceName(name, namespace))
	vsvc.SetNamespace(namespace)
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{"*"}, "spec", "hosts"); err != nil {
		return nil, fmt.Errorf("Set .spec.hosts error: %v", err)
	}

	istioGateway := os.Getenv("ISTIO_GATEWAY")
	if len(istioGateway) == 0 {
		istioGateway = "kubeflow/kubeflow-gateway"
	}
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{istioGateway},
		"spec", "gateways"); err != nil {
		return nil, fmt.Errorf("Set .spec.gateways error: %v", err)
	}

	headersRequestSet := make(map[string]string)
	// If AnnotationHeadersRequestSet is present, use its values in "headers.request.set"
	if _, ok := annotations[AnnotationHeadersRequestSet]; ok && len(annotations[AnnotationHeadersRequestSet]) > 0 {
		requestHeadersBytes := []byte(annotations[AnnotationHeadersRequestSet])
		if err := json.Unmarshal(requestHeadersBytes, &headersRequestSet); err != nil {
			// if JSON decoding fails, set an empty map
			headersRequestSet = make(map[string]string)
		}
	}
	// cast from map[string]string, as SetNestedSlice needs map[string]interface{}
	headersRequestSetInterface := make(map[string]interface{})
	for key, element := range headersRequestSet {
		headersRequestSetInterface[key] = element
	}

	// the http section of the istio VirtualService spec
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

	// add http section to istio VirtualService spec
	if err := unstructured.SetNestedSlice(vsvc.Object, http, "spec", "http"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

	return vsvc, nil

}

func (r *SparkHistoryServerReconciler) reconcileVirtualService(instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	//log := r.Log.WithValues("notebook", instance.Namespace)
	virtualService, err := generateVirtualService(instance)
	if err := ctrl.SetControllerReference(instance, virtualService, r.Scheme); err != nil {
		return err
	}
	// Check if the virtual service already exists.
	foundVirtual := &unstructured.Unstructured{}
	justCreated := false
	foundVirtual.SetAPIVersion("networking.istio.io/v1alpha3")
	foundVirtual.SetKind("VirtualService")
	err = r.Get(context.TODO(), types.NamespacedName{Name: virtualServiceName(instance.Name,
		instance.Namespace), Namespace: instance.Namespace}, foundVirtual)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Creating virtual service", "namespace", instance.Namespace, "name",
			virtualServiceName(instance.Name, instance.Namespace))
		err = r.Create(context.TODO(), virtualService)
		justCreated = true
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if !justCreated && reconcilehelper.CopyVirtualService(virtualService, foundVirtual) {
		log.Info("Updating virtual service", "namespace", instance.Namespace, "name",
			virtualServiceName(instance.Name, instance.Namespace))
		err = r.Update(context.TODO(), foundVirtual)
		if err != nil {
			return err
		}
	}

	return nil
}

func envoyFilterName(kfName string, namespace string) string {
	return fmt.Sprintf("notebook-%s-%s", namespace, kfName)
}

func generateEnvoyFilter(req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) (*unstructured.Unstructured, error) {
	name := instance.Name
	namespace := instance.Namespace

	vsvc := &unstructured.Unstructured{}
	vsvc.SetAPIVersion("networking.istio.io/v1alpha3")
	vsvc.SetKind("EnvoyFilter")
	vsvc.SetName(envoyFilterName(name, namespace))
	vsvc.SetNamespace(namespace)
	if err := unstructured.SetNestedStringSlice(vsvc.Object, []string{"*"}, "spec", "hosts"); err != nil {
		return nil, fmt.Errorf("Set .spec.hosts error: %v", err)
	}

	workloadSelector := map[string]interface{}{
		"labels": map[string]interface{}{
			"app.kubernetes.io/name":     req.Name,
			"app.kubernetes.io/instance": req.Namespace,
		},
	}
	// add workloadSelector section to istio EnvoyFilter spec
	if err := unstructured.SetNestedField(vsvc.Object, workloadSelector, "spec", "workloadSelector"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

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

	if err := unstructured.SetNestedSlice(vsvc.Object, configPatches, "spec", "configPatches"); err != nil {
		return nil, fmt.Errorf("Set .spec.http error: %v", err)
	}

	return vsvc, nil

}

func (r *SparkHistoryServerReconciler) reconcileEnvoyFilter(req ctrl.Request, instance *kubricksv1.SparkHistoryServer, log logr.Logger) error {
	//log := r.Log.WithValues("notebook", instance.Namespace)
	envoyFilter, err := generateEnvoyFilter(req, instance, log)
	if err := ctrl.SetControllerReference(instance, envoyFilter, r.Scheme); err != nil {
		return err
	}
	// Check if the envoy filter already exists.
	foundEnvoy := &unstructured.Unstructured{}
	justCreated := false
	foundEnvoy.SetAPIVersion("networking.istio.io/v1alpha3")
	foundEnvoy.SetKind("EnvoyFilter")
	err = r.Get(context.TODO(), types.NamespacedName{Name: envoyFilterName(instance.Name,
		instance.Namespace), Namespace: instance.Namespace}, foundEnvoy)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Creating envoy filter", "namespace", instance.Namespace, "name",
			envoyFilterName(instance.Name, instance.Namespace))
		err = r.Create(context.TODO(), envoyFilter)
		justCreated = true
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if !justCreated && CopyEnvoyFilter(envoyFilter, foundEnvoy) {
		log.Info("Updating envoy filter", "namespace", instance.Namespace, "name",
			envoyFilterName(instance.Name, instance.Namespace))
		err = r.Update(context.TODO(), foundEnvoy)
		if err != nil {
			return err
		}
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
					Port:       18080,
					TargetPort: intstr.FromString("historyport"),
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
		if errors.IsNotFound(err) {
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
