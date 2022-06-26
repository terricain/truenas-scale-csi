# Local install

```shell
mkdir temp
touch temp/nfs-values.yaml  # Fill with local values
helm install nfs ./charts/truenas-scale-csi --values temp/nfs-values.yaml
```

