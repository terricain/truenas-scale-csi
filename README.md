# TrueNAS Scale CSI Driver

This is a work-in-progress Kubernetes CSI driver which lets you automatically provision NFS (and eventually iSCSI) volumes hosted on a TrueNAS Scale box.

## How to install

The main Helm values you'll need to install the NFS driver would be:
```yaml
settings:
  type: "nfs"
  url: "http://192.168.69.69:1339/api/v2.0"
  nfsStoragePath: "blah"
  accessTokenSecretName: "some_existing_secret"
```

Checkout the chart [values.yaml](./charts/values.yaml) for an explanation for the nfsStoragePath and access Token values.

To install the Helm chart: (this assumes you've cloned the repo as the chart isnt hosted yet)
```shell
helm repo add truenas-scale-csi https://terrycain.github.io/truenas-scale-csi/
helm install -n kube-system -f custom-values.yaml truenas-scale-csi truenas-scale-csi/truenas-scale-csi
```
This will install the chart under the name of `truenas-scale-csi` into the `kube-system` namespace using 
custom values in a YAML file. `truenas-scale-csi/truenas-scale-csi` is the repo and chart name, all of which are called `truenas-scale-csi` :D

By default, the chart will create a `storage-class` resource which you can use to automatically provision PV's.

### Example TrueNAS access token secret

Below is some YAML to provision a secret for the CSI drvier

```yaml
apiVersion: v1
data:
  token: BASE64D_ACCESS_KEY
kind: Secret
metadata:
  namespace: default
  name: truenas-access-token
```

