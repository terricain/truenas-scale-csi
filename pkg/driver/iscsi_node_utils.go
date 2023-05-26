package driver

/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Modified to change logging
*/

import (
	"fmt"
	"os"

	"k8s.io/klog/v2"

	iscsiLib "github.com/kubernetes-csi/csi-lib-iscsi/iscsi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/utils/mount"
)

type ISCSIUtil struct{}

func (util *ISCSIUtil) AttachDisk(b iscsiDiskMounter, iscsiInfoPath string) (string, error) {
	devicePath, err := (*b.connector).Connect()
	if err != nil {
		return "", err
	}
	if devicePath == "" {
		return "", fmt.Errorf("connect reported success, but no path returned")
	}
	// Mount device
	mntPath := b.targetPath
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("heuristic determination of mount point failed:%v", err)
	}
	if !notMnt {
		klog.InfoS("iSCSI path already mounted", "mount_path", mntPath)
		return "", nil
	}

	if err := os.MkdirAll(mntPath, 0o750); err != nil {
		klog.ErrorS(err, "iSCSI failed to mkdir")
		return "", err
	}

	// Persist iscsi disk config to json file for DetachDisk path
	err = iscsiLib.PersistConnector(b.connector, iscsiInfoPath)
	if err != nil {
		klog.ErrorS(err, "failed to persist connection info, disconnecting volume and failing the publish request because persistence files are required for reliable Unpublish")
		return "", fmt.Errorf("unable to create persistence file for connection")
	}

	var options []string

	if b.readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	options = append(options, b.mountOptions...)

	err = b.mounter.FormatAndMount(devicePath, mntPath, b.fsType, options)
	if err != nil {
		klog.ErrorS(err, "iSCSI failed to mount iSCSI volume", "devicePath", devicePath, "fsType", b.fsType)
	}

	return devicePath, err
}

func (util *ISCSIUtil) DetachDisk(c iscsiDiskUnmounter, targetPath, iscsiInfoPath string) error {
	_, cnt, err := mount.GetDeviceNameFromMount(c.mounter, targetPath)
	if err != nil {
		klog.ErrorS(err, "iSCSI failed to get device from mnt", "targetPath", targetPath)
		return err
	}
	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("error checking if path exists: %v", pathErr)
	} else if !pathExists {
		klog.InfoS("iSCSI Unmount skipped because path does not exist", "targetPath", targetPath)
		return nil
	}

	klog.V(4).InfoS("loading iSCSI connection info", "iscsiInfoPath", iscsiInfoPath)
	connector, err := iscsiLib.GetConnectorFromFile(iscsiInfoPath)
	if err != nil {
		if os.IsNotExist(err) {
			klog.ErrorS(err, "assuming that ISCSI connection is already closed, ignoring")
			return nil
		}
		return status.Error(codes.Internal, err.Error())
	}
	if err = c.mounter.Unmount(targetPath); err != nil {
		klog.ErrorS(err, "iSCSI detach disk: failed to unmount", "targetPath", targetPath)
		return err
	}
	cnt--
	if cnt != 0 {
		klog.ErrorS(err, "iSCSI device is in use", "cnt", cnt)
		return nil
	}

	klog.Info(err, "detaching iSCSI device")
	err = connector.DisconnectVolume()
	if err != nil {
		klog.ErrorS(err, "iSCSI detach disk: failed to get iscsi config from path", "targetPath", targetPath)
		return err
	}

	iscsiLib.Disconnect(connector.TargetIqn, connector.TargetPortals)
	if err = os.RemoveAll(targetPath); err != nil {
		klog.ErrorS(err, "iSCSI: failed to remove mount path")
	}
	if err = os.Remove(iscsiInfoPath); err != nil {
		return err
	}

	klog.Info(err, "successfully detached ISCSI device")
	return nil
}
