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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SparkHistoryServerSpec defines the desired state of SparkHistoryServer
type SparkHistoryServerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//  Bucket name (not the full folder path) where the sparkhistoryserver should look at
	Bucket string `json:"bucket,omitempty"`

	// Container image name.
	// More info: https://kubernetes.io/docs/concepts/containers/images
	Image string `json:"image,omitempty"`

	// Image pull policy.
	// One of Always, Never, IfNotPresent.
	// More info: https://kubernetes.io/docs/concepts/containers/images#updating-images
	// +kubebuilder:default:ImagePullPolicy="IfNotPresent"
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Replicas is the number of desired replicas.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#what-is-a-replicationcontroller
	// +kubebuilder:default:Replicas=1
	Replicas *int32 `json:"replicas,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	// +kubebuilder:default:ServiceAccountName="default-editor"
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Compute Resources required by this container.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +kubebuilder:default:Resources={limits:{cpu:"1000m", memory:"1Gi"},requests:{cpu:"100m", memory:"512Mi"}}
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Configures the "cleaner" feature of sparkhistoryserver, a.k.a. logretention or logrotation
	// More info: https://spark.apache.org/docs/latest/monitoring.html#spark-history-server-configuration-options
	// +kubebuilder:default:Cleaner={enabled:true, maxAge:"30d"}
	Cleaner SparkHistoryServerCleaner `json:"cleaner,omitempty"`
}

type SparkHistoryServerCleaner struct {

	// Specifies whether the History Server should periodically clean up event logs from storage.
	// More info: https://spark.apache.org/docs/latest/monitoring.html#spark-history-server-configuration-options
	// +kubebuilder:default:Enabled=true
	Enabled bool `json:"enabled,omitempty"`

	// Job history files older than this will be deleted when the filesystem history cleaner runs.
	// More info: https://spark.apache.org/docs/latest/monitoring.html#spark-history-server-configuration-options
	// +kubebuilder:default:MaxAge="30d"
	MaxAge string `json:"maxAge,omitempty"`
}

// SparkHistoryServerStatus defines the observed state of SparkHistoryServer
type SparkHistoryServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SparkHistoryServer is the Schema for the sparkhistoryservers API
type SparkHistoryServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SparkHistoryServerSpec   `json:"spec,omitempty"`
	Status SparkHistoryServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SparkHistoryServerList contains a list of SparkHistoryServer
type SparkHistoryServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SparkHistoryServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SparkHistoryServer{}, &SparkHistoryServerList{})
}
