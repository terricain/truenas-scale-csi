# TrueNAS Scale CSI Driver

This is a work-in-progress Kubernetes CSI driver which lets you automatically provision
NFS and eventually iSCSI volumes hosted on a TrueNAS Scale box.

**Breaking change** TrueNAS-22.12.4.2 and later contain a breaking change in the API, whereby creating and listing NFS 
shares now returns a mountpoint `path` string instead of `paths` which was a list of strings. Normally this wouldn't be
an issue, but it seems the API version of 2.0 hasn't been bumped so some patches have been put in place to account for
this. If you're experiencing issues with NFS dynamic PVs then update to chart version 0.5.0.

## How to install

The main Helm values you'll need to install the NFS driver would be:
```yaml
settings:
  type: "nfs"
  url: "http://192.168.69.69:1339/api/v2.0"
  nfsStoragePath: "blah"
  accessTokenSecretName: "some_existing_secret"
```

Installing the iSCSI driver is near enough identical.

Checkout the chart [values.yaml](./charts/truenas-scale-csi/values.yaml) for an explanation for the nfs/iSCSI StoragePath and access Token values.

To install the Helm chart: (this assumes you've cloned the repo as the chart isnt hosted yet)
```shell
helm repo add truenas-scale-csi https://terricain.github.io/truenas-scale-csi/
helm install -n kube-system -f custom-values.yaml nfs truenas-scale-csi/truenas-scale-csi
helm install -n kube-system -f custom-values.yaml iscsi truenas-scale-csi/truenas-scale-csi
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

## Roadmap

A vague TODO list of features I hope to implement

* Increase logging of GRPC requests
* Volume expansion
* Volume snapshots
