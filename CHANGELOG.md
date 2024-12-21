# truenas-csi Releases

## 1.2.0 - 21-12-2024

* Removed volume size limit.
* Fixed potential bug when access error values if its nil.

## 1.1.1 - 15-09-2024

* Fixed nil pointer dereference when creating NFS volumes.

## 1.1.0 - 08-07-2024

* Fixed CSI driver name issues.

## 1.0.0 - 21-06-2024

* Package rename & updates.

## 0.4.0 - 26-05-2023

* Removed `-log-level` flag in favour of `-v`.
* Fixed populating `version`. 

## 0.3.0 - 26-05-2023

* Swapped out zerolog for klog to be more consistent with other CSI drivers.
* Updated container base image.
* Updated go version and kubernetes dependencies. 

## 0.2.0 - 07-08-2022

* Added `--ignore-tls` flag.

## 0.1.0 - 06-08-2022

* Initial release.

# Chart Releases

## chart-0.3.0 - 26-05-2023

* Added and bumped timeout for CSI sidecars.
* Added `fsGroupPolicy` to CSIDriver resource.
* Bumped sidecar versions.
* Migrated from `k8s.gcr.io` to `registry.k8s.io`.
* Configured security contexts.

## chart-0.2.1 - 07-08-2022

* Fixed erroneous `eq` in comparison.

## chart-0.2.0 - 07-08-2022

* Added `.Values.settings.ignoreTLS` flag into controller args.

## chart-0.1.1 - 06-08-2022

* Initial release.
