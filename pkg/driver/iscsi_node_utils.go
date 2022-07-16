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
	"github.com/rs/zerolog/log"
	"os"

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
		log.Info().Str("mount_path", mntPath).Msg("iSCSI path already mounted")
		return "", nil
	}

	if err := os.MkdirAll(mntPath, 0o750); err != nil {
		log.Error().Err(err).Msg("iSCSI failed to mkdir")
		return "", err
	}

	// Persist iscsi disk config to json file for DetachDisk path
	err = iscsiLib.PersistConnector(b.connector, iscsiInfoPath)
	if err != nil {
		log.Error().Err(err).Msg("failed to persist connection info, disconnecting volume and failing the publish request because persistence files are required for reliable Unpublish")
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
		log.Error().Err(err).Str("devicePath", devicePath).
			Str("fsType", b.fsType).Str("mntPath", mntPath).Msg("iSCSI failed to mount iSCSI volume")
	}

	return devicePath, err
}

func (util *ISCSIUtil) DetachDisk(c iscsiDiskUnmounter, targetPath, iscsiInfoPath string) error {
	_, cnt, err := mount.GetDeviceNameFromMount(c.mounter, targetPath)
	if err != nil {
		log.Error().Err(err).Str("targetPath", targetPath).Msg("iSCSI failed to get device from mnt")
		return err
	}
	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("error checking if path exists: %v", pathErr)
	} else if !pathExists {
		log.Warn().Str("targetPath", targetPath).Msg("iSCSI Unmount skipped because path does not exist")
		return nil
	}

	log.Info().Str("iscsiInfoPath", iscsiInfoPath).Msg("Loading iSCSI connection info")
	connector, err := iscsiLib.GetConnectorFromFile(iscsiInfoPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Err(err).Msg("assuming that ISCSI connection is already closed")
			return nil
		}
		return status.Error(codes.Internal, err.Error())
	}
	if err = c.mounter.Unmount(targetPath); err != nil {
		log.Error().Err(err).Str("targetPath", targetPath).Msg("iSCSI detach disk: failed to unmount")
		return err
	}
	cnt--
	if cnt != 0 {
		log.Error().Int("cnt", cnt).Msg("iSCSI the device is in use")
		return nil
	}

	log.Info().Msg("detaching iSCSI device")
	err = connector.DisconnectVolume()
	if err != nil {
		log.Error().Err(err).Str("targetPath", targetPath).Msg("iSCSI detach disk: failed to get iscsi config from path")
		return err
	}

	iscsiLib.Disconnect(connector.TargetIqn, connector.TargetPortals)
	if err := os.RemoveAll(targetPath); err != nil {
		log.Error().Err(err).Msg("iSCSI: failed to remove mount path")
	}
	err = os.Remove(iscsiInfoPath)
	if err != nil {
		return err
	}

	log.Info().Msg("successfully detached ISCSI device")
	return nil
}
