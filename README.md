# Spark-history-server-controller

## Important files

- controllers/sparkhistoryserver_controller.go
- api/v1/sparkhistoryserver_types.go
- config/rbac/sparkhistoryserver_editor_role.yaml (needed to be added in config/rbac/kustomization.yaml)

## Development process
```
make docker-build docker-push IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
make deploy IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
```


## Testing



## Kube-builder tutorial

### Prerequisites

- Minikube can be a good fit (other solutions are `kind` or `k3s`) to develop locally (no worries to destroy the production system but if it need too many dependencies, it is very time consuming to integrate it into Minikube):
  - How to install Minikube, can be found [here](https://minikube.sigs.k8s.io/docs/start/)
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
apiVersion: kubricks.kubricks.io/v1
kind: SparkHistoryServer
metadata:
  name: sparkhistoryserver
  namespace: default
```

More info about how to get started with kubebuilder can be found [here](https://github.com/kubernetes-sigs/kubebuilder#getting-started) and [here](https://book.kubebuilder.io/quick-start.html).

[Create a Project:](https://book.kubebuilder.io/quick-start.html#create-a-project)
```
kubebuilder init --domain kubricks.io --repo kubricks.io/sparkhistoryserver
```
What matters?
- `--domain kubricks.io` = `apiVersion: kubricks.<here>/v1`
- `--repo kubricks.io/sparkhistoryserver` = `module <here>` in `go.mod`


[Create an API:](https://book.kubebuilder.io/quick-start.html#create-an-api)
```
kubebuilder create api --group kubricks --version v1 --kind SparkHistoryServer

make manifests
```
What matters?
- `--group kubricks` = `apiVersion: <here>.kubricks.io/v1`
- `--version v1` = `apiVersion: <here>.kubricks.io/<here>`
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





