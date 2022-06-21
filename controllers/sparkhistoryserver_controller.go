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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/resource"
	kubricksv1 "kubricks.io/sparkhistoryserver/api/v1"
)

// SparkHistoryServerReconciler reconciles a SparkHistoryServer object
type SparkHistoryServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubricks.kubricks.io,resources=sparkhistoryservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=api.core.v1,resources=services,verbs=get;list;watch;create;update;patch;delete

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

		// Delete service if it exists
		var sparkhistoryserverService corev1.Service
		if err := r.Get(ctx, req.NamespacedName, &sparkhistoryserverService); err == nil {
			return r.RemoveService(ctx, &sparkhistoryserverService, log)
		}

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
	// Ensure that at least minimum number of replicas are maintained

	// Ensure that the service is created for the sparkhistoryserver
	log.Info("Ensuring Service is created", "sparkhistoryserver", req.NamespacedName)
	var sparkhistoryserverService corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &sparkhistoryserverService); err != nil {
		log.Info("unable to fetch Deployment for sparkhistoryserver", "sparkhistoryserver", req.NamespacedName)
		// Create the service
		return r.CreateService(ctx, req, sparkhistoryserver, log)
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
	//startcommand := "export SPARK_HISTORY_OPTS=\"$SPARK_HISTORY_OPTS -Dspark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem -Dspark.ui.proxyBase=/sparkhistory/salil-mishra -Dspark.ui.reverseProxy=true -Dspark.ui.reverseProxyUrl=https://kubeflow.at.onplural.sh/sparkhistory/salil-mishra -Dspark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.WebIdentityTokenCredentialsProvider -Dspark.history.fs.logDirectory=s3a://at-plural-sh-at-onplural-sh-kubeflow-pipelines/pipelines/salil-mishra/history\"; /opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;"

	startcommand := "export SPARK_HISTORY_OPTS=\"$SPARK_HISTORY_OPTS"
	startcommand += " -Dspark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem"
	//	startcommand += " -Dspark.ui.proxyBase=/sparkhistory/salil-mishra"
	//	startcommand += " -Dspark.ui.reverseProxy=true"
	//	startcommand += " Dspark.ui.reverseProxyUrl=https://kubeflow.at.onplural.sh/sparkhistory/salil-mishra"
	//	startcommand += " -Dspark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.WebIdentityTokenCredentialsProvider"
	startcommand += " -Dspark.hadoop.fs.s3a.access.key=AKIA4SDN3Y37NY4UADGE"
	startcommand += " -Dspark.hadoop.fs.s3a.secret.key=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"
	startcommand += " -Dspark.history.fs.logDirectory=s3a://tims-delta-lake/history\";"
	//startcommand += " -Dspark.history.fs.logDirectory=s3a://at-plural-sh-at-onplural-sh-kubeflow-pipelines/pipelines/salil-mishra/history\";"
	startcommand += " /opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;"

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
			//Replicas: sparkhistoryserver.Spec.MinReplica,
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
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default-editor",
					Containers: []corev1.Container{
						corev1.Container{
							Name: sparkhistoryserver.Name,
							Env: []corev1.EnvVar{
								{
									Name:  "SPARK_NO_DAEMONIZE",
									Value: "false",
								},
							},
							Image: "public.ecr.aws/atcommons/sparkhistoryserver:14469", //sparkhistoryserver.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 18080, //sparkhistoryserver.Spec.Port,
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1000m"), //sparkhistoryserver.Spec.CPURequest, // https://github.com/GoogleCloudPlatform/flink-on-k8s-operator/blob/master/controllers/flinkcluster_converter_test.go
								},
							},
							Command: []string{
								"/bin/sh",
								"-c",
								startcommand,
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
					TargetPort: intstr.FromInt(18080), //TODO historyport
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
