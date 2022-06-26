# Local install

Deploy the CSI provisioner
```shell
mkdir temp
touch temp/nfs-values.yaml  # Fill with local values
helm install nfs ./charts/truenas-scale-csi --values temp/nfs-values.yaml
```

Create a PVC
```shell
cat <<EOF >temp/pvc.yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-nfs-claim
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: truenas-nfs
  resources:
    requests:
      storage: 5Gi
EOF
kubectl apply -f temp/pvc.yaml
```

Create a pod which mounts the PVC, specify a node selector for easier debugging
```shell
cat <<EOF >temp/pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: app
spec:
  nodeSelector:
    kubernetes.io/hostname: kube02
  containers:
  - name: app
    image: centos
    command: ["/bin/sh"]
    args: ["-c", "while true; do echo $(date -u) >> /data/out.txt; sleep 5; done"]
    volumeMounts:
    - name: persistent-storage
      mountPath: /data
  volumes:
  - name: persistent-storage
    persistentVolumeClaim:
      claimName: test-nfs-claim
EOF
kubectl apply -f temp/pod.yaml
```

Exec into the pod, check the mount, check size of file, kill pod, recreate pod and check size of file again, it should have
persisted :)
```shell
$ kubectl exec -it app bash
> ls -l /data
> cat /data/out.txt
$ kubectl delete pod app
$ kubectl apply -f temp/pod.yaml
$ kubectl exec -it app bash
> ls -l /data
```