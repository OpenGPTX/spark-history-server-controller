# Spark-history-server-controller

A lot of background information can be found [here](https://github.com/KubeSoup/spark-history-server). This  spark-history-server-controller is just the automation from it.
## Important files

- `controllers/sparkhistoryserver_controller.go` this is the most important file as it does all the magic and reconciles the following resources:
  - `Deployment`
  - `Service`
  - `VirtualService`
  - `EnvoyFilter`
- `api/v1/sparkhistoryserver_types.go` this represents the CRD
- `config/rbac/sparkhistoryserver_editor_role.yaml` needed to be added in `config/rbac/kustomization.yaml` including adding the following that the ServiceAccounts `default-editor` have permissions to deal with the CR `SparkHistoryServer`:
```
  labels:
    rbac.authorization.kubeflow.org/aggregate-to-kubeflow-edit: "true"
```

## Development process

In case you change the CRD (`api/v1/sparkhistoryserver_types.go`), you need to generate those:
```
make manifest

# If you want to deploy only CRD's (but "make deploy" does the same too):
make install
```

In order to build and deploy:
```
make docker-build docker-push IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
make deploy IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
```


## Testing

1. Creating a SparkHistoryServer:
```
cat <<EOF | kubectl apply -f -
apiVersion: platform.kubesoup.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: <your_namespace>
spec:
  image: public.ecr.aws/atcommons/sparkhistoryserver:14469 #It is Spark version 3.2.1
EOF
```

2. Look that the default values are set correctly `kubectl get SparkHistoryServer sparkhistoryserver -o yaml`:
```
apiVersion: platform.kubesoup.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: <your_namespace>
spec:
  cleaner:
    enabled: true
    maxAge: 30d
  image: public.ecr.aws/atcommons/sparkhistoryserver:14469
  imagePullPolicy: IfNotPresent
  replicas: 1
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 100m
      memory: 512Mi
  serviceAccountName: default-editor
```

3. Ensure everything is set up correctly and you can access it with the according url: `https://kubeflow.at.onplural.sh/sparkhistory/tim-krause`
```
kubectl get deployment sparkhistoryserver -o yaml -n <your_namespace>
kubectl get service sparkhistoryserver -o yaml -n <your_namespace>
kubectl get virtualservice sparkhistoryserver -o yaml -n <your_namespace>
kubectl get envoyfilter sparkhistoryserver -o yaml -n <your_namespace>
```

4. You can also adjust the `SparkHistoryServer` and the controller reconciles it accordingly.

5. Ensure everything gets cleaned up after deleting the CR: `kubectl delete SparkHistoryServer sparkhistoryserver`

## Build Dockerimage with new official version

Adjust the version!
```
make docker-build docker-push IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:0.1.0
```

Update our [rollout doc](https://github.com/KubeSoup/internal-docs/blob/main/plural/cluster-setup.md#install-sparkhistoryserver) (the version) and do the rollout like it is described there!

## Kube-builder tutorial

### Prerequisites

- Minikube can be a good fit (other solutions are `kind` or `k3s`) to develop locally (no worries to destroy the production system but if it need too many dependencies, it is very time consuming to integrate it into Minikube):
  - How to install Minikube, can be found 
  - Each day, you just start Minikube: `minikube start` (this also configures kubeconfig automatically)
  - In case you need the kubeconfig again without restarting Minikube: `minikube update-context`
- Install [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder/releases) (latest was 3.4.1 at that time):
```
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.4.1/kubebuilder_linux_amd64

chmod +x kubebuilder_linux_amd64
sudo mv kubebuilder_linux_amd64 /usr/local/bin/kubebuilder

kubebuilder version
```
- Install [Golang](https://github.com/golang/go/releases) (I decided to go for v1.17+): 
```
wget https://go.dev/dl/go1.17.11.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.17.11.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc

go version
```


### Some important documentation for Golang@K8s

Especially when writing the CRD or creating K8s objects, the official documentation helps a lot in terms of (data) types, structs and so on.

- Here is the spec of a [`Deployment`](https://pkg.go.dev/k8s.io/api/apps/v1#Deployment)
- If you go deeper and want to take a look into the `ObjectMeta` (a.k.a `metadata`), look [here](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta)
- The so called `ServicePort` (a.k.a `port` in a Service) can be found [here](https://pkg.go.dev/k8s.io/api/core/v1#ServicePort)

Click around, google specific K8s resources and you will find the according doc that will help you.

### Getting started

We assume, we want to achieve the following CR:
```
apiVersion: platform.kubesoup.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: default
```

More info about how to get started with kubebuilder can be found [here](https://github.com/kubernetes-sigs/kubebuilder#getting-started) and [here](https://book.kubebuilder.io/quick-start.html).

[Create a Project:](https://book.kubebuilder.io/quick-start.html#create-a-project)
```
kubebuilder init --domain kubesoup.io --repo kubesoup.io/sparkhistoryserver
```
What matters?
- `--domain kubesoup.io` = `apiVersion: kubesoup.<here>/v1`
- `--repo kubesoup.io/sparkhistoryserver` = `module <here>` in `go.mod`


[Create an API:](https://book.kubebuilder.io/quick-start.html#create-an-api)
```
kubebuilder create api --group kubesoup --version v1 --kind SparkHistoryServer

make manifests
```
What matters?
- `--group kubesoup` = `apiVersion: <here>.kubesoup.io/v1`
- `--version v1` = `apiVersion: <here>.kubesoup.io/<here>`
- `--kind SparkHistoryServer` = `kind: <here>`


[Test It Out:](https://book.kubebuilder.io/quick-start.html#test-it-out)

```
make install # Installs CRD(s)

make run # Runs the controller locally in the terminal
```

[Install Instances of Custom Resources:](https://book.kubebuilder.io/quick-start.html#install-instances-of-custom-resources)

```
# Deploy a CR via:
kubectl apply -f config/samples/

kubectl get sparkhistoryserver -A
```

[Run It On the Cluster:](https://book.kubebuilder.io/quick-start.html#run-it-on-the-cluster)

```
make docker-build docker-push IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev

make deploy IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
```

[Uninstall CRDs](https://book.kubebuilder.io/quick-start.html#uninstall-crds)
 and [Undeploy controller:](https://book.kubebuilder.io/quick-start.html#undeploy-controller)

```
make uninstall

make undeploy
```

### Understanding some important mechanisms

#### RBAC 
It is important to grant according permissions for what the controller reconciles! In `controllers/sparkhistoryserver_controller.go` e.g. add for example permission for `deployments` (the `//` are important and do magic behind):
```
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
```
To make an affect on your change, do `make deploy`. You will see that it added the according permissions in `config/rbac/role.yaml`.
#### CRD 

- defaults: for setting defaults in the CR if the user does not specifies it (makes it convenient for the users), just add the according "comments" in `api/v1/sparkhistoryserver_types.go`:
```
	// +kubebuilder:default:Replicas=1
```
That sets the `Replicas` in the CR to `1` if it is not specified. If you specify it, of course it overwrites the default value:
```
apiVersion: platform.kubesoup.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: <your_namespace>
spec:
  image: public.ecr.aws/atcommons/sparkhistoryserver:14469 #It is Spark version 3.2.1
  replicas: 2
```

- not using `omitempty`: `omitempty` means you can leave it empty in the CR. This could cause issues and it might be helpful to force a specified value by the user in the CR. E.g. if you want to enforce a `spec.image` you can do so by deleting `omitempty` for the following lines:
```
	Spec   SparkHistoryServerSpec   `json:"spec"`

...

	Image string `json:"image"`
```
If the user does not specify `spec.image` in the CR:
```
cat <<EOF | kubectl apply -f -
apiVersion: platform.kubesoup.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: <your_namespace>
spec:
  replicas: 1
EOF
```
then the use will get an error:
```
error: error validating "STDIN": error validating data: ValidationError(SparkHistoryServer.spec): missing required field "image" in io.kubesoup.kubesoup.v1.SparkHistoryServer.spec; if you choose to ignore these errors, turn validation off with --validate=false
```


