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
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/gogo/protobuf/types"

	// "google.golang.org/protobuf/proto"
	istioNetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	istioNetworking "istio.io/api/networking/v1beta1"
	istioNetworkingClientv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioNetworkingClient "istio.io/client-go/pkg/apis/networking/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubesoupv1 "kubesoup.io/sparkhistoryserver/api/v1"
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

//+kubebuilder:rbac:groups=platform.kubesoup.io,resources=sparkhistoryservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.kubesoup.io,resources=sparkhistoryservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.kubesoup.io,resources=sparkhistoryservers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
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
	var sparkHistoryServer kubesoupv1.SparkHistoryServer
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
	if err := r.reconcileVirtualService(ctx, req, &sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile EnvoyFilter.
	if err := r.reconcileEnvoyFilter(ctx, req, &sparkHistoryServer, log); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SparkHistoryServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubesoupv1.SparkHistoryServer{}).
		Complete(r)
}

// Generates EnvoyFilter from CR SparkHistoryServer
func generateEnvoyFilter(instance *kubesoupv1.SparkHistoryServer) (*istioNetworkingClientv1alpha3.EnvoyFilter, error) {
	name := instance.Name
	namespace := instance.Namespace
	var inlineCode = "function envoy_on_response(response_handle, context)\n    response_handle:headers():replace(\"location\", \"\");\nend\n"

	envoyFilter := &istioNetworkingClientv1alpha3.EnvoyFilter{

		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: istioNetworkingv1alpha3.EnvoyFilter{
			ConfigPatches: []*istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
				{
					ApplyTo: istioNetworkingv1alpha3.EnvoyFilter_HTTP_FILTER,
					Match: &istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
						Context: istioNetworkingv1alpha3.EnvoyFilter_SIDECAR_INBOUND,
						ObjectTypes: &istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
							Listener: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch{
								FilterChain: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
									Filter: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
										Name: "envoy.filters.network.http_connection_manager",
										SubFilter: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch{
											Name: "envoy.filters.http.router",
										},
									},
								},
							},
						},
					},
					// we use replace on the header so that existing values are overwritten.
					Patch: &istioNetworkingv1alpha3.EnvoyFilter_Patch{
						Operation: istioNetworkingv1alpha3.EnvoyFilter_Patch_INSERT_BEFORE,
						Value: &types.Struct{
							Fields: map[string]*types.Value{
								"name": {
									Kind: &types.Value_StringValue{
										StringValue: "envoy.lua",
									},
								},
								"typed_config": {
									Kind: &types.Value_StructValue{
										StructValue: &types.Struct{
											Fields: map[string]*types.Value{
												"@type": {
													Kind: &types.Value_StringValue{
														StringValue: "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua",
													},
												},
												"inlineCode": {
													Kind: &types.Value_StringValue{
														StringValue: inlineCode,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			WorkloadSelector: &istioNetworkingv1alpha3.WorkloadSelector{
				Labels: map[string]string{
					"app.kubernetes.io/name":     name,
					"app.kubernetes.io/instance": namespace,
				},
			},
		},
	}

	// envoyFilter := &istioNetworkingClientv1alpha3.EnvoyFilter{

	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      name,
	// 		Namespace: namespace,
	// 	},
	// 	Spec: istioNetworkingv1alpha3.EnvoyFilter{
	// 		ConfigPatches: []*istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
	// 			{
	// 				ApplyTo: istioNetworkingv1alpha3.EnvoyFilter_HTTP_FILTER,
	// 				Match: &istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
	// 					Context: istioNetworkingv1alpha3.EnvoyFilter_SIDECAR_INBOUND,
	// 					ObjectTypes: &istioNetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
	// 						Listener: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch{
	// 							FilterChain: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
	// 								Filter: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
	// 									Name: "envoy.filters.network.http_connection_manager",
	// 									SubFilter: &istioNetworkingv1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch{
	// 										Name: "envoy.filters.http.router",
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 				// we use replace on the header so that existing values are overwritten.
	// 				Patch: &istioNetworkingv1alpha3.EnvoyFilter_Patch{
	// 					Operation: istioNetworkingv1alpha3.EnvoyFilter_Patch_INSERT_BEFORE,
	// 					Value: &_struct.Struct{
	// 						Fields: map[string]*structpb.Value{
	// 							"name": {
	// 								Kind: &structpb.Value_StringValue{
	// 									StringValue: "envoy.lua",
	// 								},
	// 							},
	// 							"typed_config": {
	// 								Kind: &structpb.Value_StructValue{
	// 									StructValue: &_struct.Struct{
	// 										Fields: map[string]*structpb.Value{
	// 											"@type": {
	// 												Kind: &structpb.Value_StringValue{
	// 													StringValue: "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua",
	// 												},
	// 											},
	// 											"inlineCode": {
	// 												Kind: &structpb.Value_StringValue{
	// 													StringValue: inlineCode,
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 		WorkloadSelector: &istioNetworkingv1alpha3.WorkloadSelector{
	// 			Labels: map[string]string{
	// 				"app.kubernetes.io/name":     name,
	// 				"app.kubernetes.io/instance": namespace,
	// 			},
	// 		},
	// 	},
	// }

	return envoyFilter, nil
}

func (r *SparkHistoryServerReconciler) reconcileEnvoyFilter(ctx context.Context, req ctrl.Request, instance *kubesoupv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating EnvoyFilter")
	envoyFilter, err := generateEnvoyFilter(instance)
	if err := ctrl.SetControllerReference(instance, envoyFilter, r.Scheme); err != nil {
		return err
	}
	// Check if the EnvoyFilter already exists.
	var foundEnvoyFilter istioNetworkingClientv1alpha3.EnvoyFilter
	if err = r.Get(ctx, req.NamespacedName, &foundEnvoyFilter); err != nil {
		if apierrs.IsNotFound(err) {
			if err = r.Create(ctx, envoyFilter); err != nil {
				return err
			}
			log.Info("EnvoyFilter created")
		} else {
			log.Info("Failed to get EnvoyFilter")
			return err
		}
	} else if !reflect.DeepEqual(envoyFilter.Spec, foundEnvoyFilter.Spec) {
		envoyFilter.ObjectMeta = foundEnvoyFilter.ObjectMeta
		if err = r.Update(context.TODO(), envoyFilter); err != nil {
			return err
		}
		log.Info("EnvoyFilter updated")
	}

	return nil
}

// Generates Service from CR SparkHistoryServer
func generateService(instance *kubesoupv1.SparkHistoryServer) (*corev1.Service, error) {
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

func (r *SparkHistoryServerReconciler) reconcileService(ctx context.Context, req ctrl.Request, instance *kubesoupv1.SparkHistoryServer, log logr.Logger) error {
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
func generateDeployment(instance *kubesoupv1.SparkHistoryServer, bucketName string) (*appsv1.Deployment, error) {

	var sparkhistoryserverDeployment *appsv1.Deployment
	var historyCommand = "export SPARK_HISTORY_OPTS=\"$SPARK_HISTORY_OPTS \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem \\\n"
	historyCommand += "  -Dspark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.WebIdentityTokenCredentialsProvider \\\n"
	historyCommand += "  -Dspark.history.fs.logDirectory=s3a://" + bucketName + "/pipelines/" + instance.Namespace + "/history \\\n"
	historyCommand += "  -Dspark.ui.proxyBase=/sparkhistory/" + instance.Namespace + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.enabled=" + strconv.FormatBool(instance.Spec.Cleaner.Enabled) + " \\\n"
	historyCommand += "  -Dspark.history.fs.cleaner.maxAge=" + instance.Spec.Cleaner.MaxAge + "\";\n"
	historyCommand += "/opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;\n"

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

func (r *SparkHistoryServerReconciler) reconcileDeployment(ctx context.Context, req ctrl.Request, instance *kubesoupv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating Deployment")

	// Get Bucket name from K8s cluster if it is not defined in the CRD
	var bucketName = ""
	if instance.Spec.Bucket != "" {
		bucketName = instance.Spec.Bucket
	} else {
		var configmap corev1.ConfigMap
		var reqConfigmap = req
		reqConfigmap.NamespacedName.Name = "artifact-repositories"
		if err := r.Get(ctx, reqConfigmap.NamespacedName, &configmap); err != nil {
			return err
		}
		var configmapData = configmap.Data["default-v1"]
		regex := regexp.MustCompile(`bucket: (.*)\n\s\s`)
		var regexResult = regex.FindAllStringSubmatch(configmapData, -1)
		if len(regexResult) > 0 && len(regexResult[0]) > 1 {
			bucketName = regexResult[0][1]
		} else {
			return errors.New("No bucket name found. Cannot proceed!")
		}
	}

	deployment, err := generateDeployment(instance, bucketName)
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

// Generates VirtualService from CR SparkHistoryServer
func generateVirtualService(instance *kubesoupv1.SparkHistoryServer) (*istioNetworkingClient.VirtualService, error) {

	name := instance.Name
	namespace := instance.Namespace
	clusterDomain := "cluster.local"
	prefix := fmt.Sprintf("/sparkhistory/%s", namespace)
	rewrite := "/"

	if clusterDomainFromEnv, ok := os.LookupEnv("CLUSTER_DOMAIN"); ok {
		clusterDomain = clusterDomainFromEnv
	}
	service := fmt.Sprintf("%s.%s.svc.%s", name, namespace, clusterDomain)

	httpRoutes := []*istioNetworking.HTTPRoute{}

	httpRoute := &istioNetworking.HTTPRoute{
		Match: []*istioNetworking.HTTPMatchRequest{
			{
				Uri: &istioNetworking.StringMatch{
					MatchType: &istioNetworking.StringMatch_Prefix{
						Prefix: prefix + "/",
					},
				},
			},
			{
				Uri: &istioNetworking.StringMatch{
					MatchType: &istioNetworking.StringMatch_Prefix{
						Prefix: prefix,
					},
				},
			},
		},
		Rewrite: &istioNetworking.HTTPRewrite{
			Uri: rewrite,
		},
		Route: []*istioNetworking.HTTPRouteDestination{
			{
				Destination: &istioNetworking.Destination{
					Host: service,
					Port: &istioNetworking.PortSelector{
						Number: uint32(DefaultServingPort),
					},
				},
			},
		},
	}
	httpRoutes = append(httpRoutes, httpRoute)

	// The gateway section of the istio VirtualService spec
	istioGateway := os.Getenv("ISTIO_GATEWAY")
	if len(istioGateway) == 0 {
		istioGateway = "kubeflow/kubeflow-gateway"
	}

	virtualservice := &istioNetworkingClient.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: istioNetworking.VirtualService{
			Hosts: []string{
				"*",
			},
			Gateways: []string{
				istioGateway,
			},
			Http: httpRoutes,
		},
	}

	return virtualservice, nil
}

func (r *SparkHistoryServerReconciler) reconcileVirtualService(ctx context.Context, req ctrl.Request, instance *kubesoupv1.SparkHistoryServer, log logr.Logger) error {
	log.Info("Updating VirtualService")
	virtualService, err := generateVirtualService(instance)
	if err := ctrl.SetControllerReference(instance, virtualService, r.Scheme); err != nil {
		return err
	}
	// Check if the VirtualService already exists.
	var foundVirtualservice istioNetworkingClient.VirtualService
	if err = r.Get(ctx, req.NamespacedName, &foundVirtualservice); err != nil {
		if apierrs.IsNotFound(err) {
			if err = r.Create(ctx, virtualService); err != nil {
				return err
			}
			log.Info("VirtualService created")
		} else {
			log.Info("Failed to get VirtualService")
			return err
		}
	} else if !reflect.DeepEqual(virtualService.Spec, foundVirtualservice.Spec) {
		// if proto.Equal(virtualService.Spec, foundVirtualservice.Spec) {
		//  print("huhu")
		// }
		virtualService.ObjectMeta = foundVirtualservice.ObjectMeta
		if err = r.Update(context.TODO(), virtualService); err != nil {
			return err
		}
		log.Info("VirtualService updated")
	}

	return nil
}
