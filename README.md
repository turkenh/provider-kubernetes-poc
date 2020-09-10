# [POC] provider-kubernetes

`provider-kubernetes` is a Crossplane Provider that enables deployment and management
of Kubernetes Resources on remote Kubernetes clusters typically provisioned by Crossplane:

- A `Provider` resource type that only points to a credentials `Secret`.
- A `KubernetesResource` resource type that is to manage Kubernetes Resources.
- A managed resource controller that reconciles `KubernetesResource` objects and manages Kubernetes Resources.

## Install

If you would like to install `provider-kubernetes` without modifications create
the following `ClusterPackageInstall` in a Kubernetes cluster where Crossplane is
installed:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: helm
---
apiVersion: packages.crossplane.io/v1alpha1
kind: ClusterPackageInstall
metadata:
  name: provider-kubernetes
  namespace: helm
spec:
  package: "crossplane-contrib/provider-kubernetes:latest"
```

## Developing locally

Start a local development environment with Kind where `crossplane` is installed:

```
make local-dev
```

Run controller against the cluster:

```
make run
```

Since controller is running outside of the Kind cluster, you need to make api server accessible (on a separate terminal):

```
sudo kubectl proxy --port=80
```

### Testing in Local Cluster

1. Deploy [RBAC for local cluster](examples/provider-config/local-service-account.yaml)

    ```
    kubectl apply -f examples/provider-config/local-service-account.yaml
    ```
1. Deploy [local-provider.yaml](examples/provider-config/local-provider-config.yaml) by replacing `spec.credentialsSecretRef.name` with the token secret name.

    ```
    EXP="s/<kubernetes-provider-token-secret-name>/$(kubectl get sa kubernetes-provider -n crossplane-system -o jsonpath="{.secrets[0].name}")/g"
    cat examples/provider-config/local-provider-config.yaml | sed -e "${EXP}" | kubectl apply -f -
    ```
1. Now you can create `KubernetesResource` resources with provider reference, see [sample release.yaml](examples/sample/release.yaml).

### Cleanup

```
make local.down
```
