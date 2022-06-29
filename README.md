# spark-history-server-controller

## kube-builder-tutorial

### Prerequisites

- Minikube can be a good fit to develop locally (no worries to destroy the production system but if it need to many dependencies, it is very time consuming to integrate it into minikube):
 - How to install minikube, can be found here: https://minikube.sigs.k8s.io/docs/start/
 - Each day, you just start minikube: `minikube start` (this also configures kubeconfig automatically)
 - In case you need the kubeconfig again without restarting minikube: `minikube update-context`
- Install kubebuilder (latest was 3.4.1 at that time): https://github.com/kubernetes-sigs/kubebuilder/releases
```
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.4.1/kubebuilder_linux_amd64

chmod +x kubebuilder_linux_amd64
sudo mv kubebuilder_linux_amd64 /usr/local/bin/kubebuilder

kubebuilder version
```
- Install Golang (I decided to go with v1.17+): https://github.com/golang/go/releases
```
wget https://go.dev/dl/go1.17.11.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.17.11.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc
go version
```


### Some important doc for Golang@K8s

Especially when writing the CRD or creating K8s objects, the official documentation help a lot in terms of (data) types, structs and so on.

- Here is the spec of a `Deployment`: https://pkg.go.dev/k8s.io/api/apps/v1#Deployment
- If you go deeper and want to take a look into the `ObjectMeta` (a.k.a `metadata`), look here: https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta
- The so called `ServicePort` (a.k.a `port` in a Service) can be found here: https://pkg.go.dev/k8s.io/api/core/v1#ServicePort

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

More info about how to get started with kubebuilder can be found here: https://github.com/kubernetes-sigs/kubebuilder#getting-started and here: https://book.kubebuilder.io/quick-start.html

https://book.kubebuilder.io/quick-start.html#create-a-project
```
kubebuilder init --domain kubricks.io --repo kubricks.io/sparkhistoryserver
```
What matters?
- `--domain kubricks.io` = `apiVersion: kubricks.<here>/v1`
- `--repo kubricks.io/sparkhistoryserver` = `module <here>` in `go.mod`


https://book.kubebuilder.io/quick-start.html#create-an-api
```
kubebuilder create api --group kubricks --version v1 --kind SparkHistoryServer

make manifests
```
What matters?
- `--group kubricks` = `apiVersion: kubricks.kubricks.io/v1`
- `--version v1` = `apiVersion: <here>.kubricks.io/<here>`
- `--kind SparkHistoryServer` = `kind: <here>`


https://book.kubebuilder.io/quick-start.html#test-it-out
```
make install #installs CRD(s)

make run #runs the controller locally in the terminal
```

https://book.kubebuilder.io/quick-start.html#install-instances-of-custom-resources
```
# Deploy a CR via:
kubectl apply -f config/samples/

kubectl get sparkhistoryserver -A
```

https://book.kubebuilder.io/quick-start.html#run-it-on-the-cluster
```
make docker-build docker-push IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev

make deploy IMG=public.ecr.aws/atcommons/sparkhistoryservercontroller:dev
```

https://book.kubebuilder.io/quick-start.html#uninstall-crds and https://book.kubebuilder.io/quick-start.html#undeploy-controller
```
make uninstall

make undeploy
```