package main

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

func main() {
	conn, err := grpc.Dial("unix:///tmp/csi.sock", grpc.WithInsecure())
	if err != nil {
		klog.ErrorS(err, "failed to listen to socket")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	defer conn.Close()

	controllerClient := csi.NewControllerClient(conn)

	//resp, err := controllerClient.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp).Msg("")
	//}

	//
	//resp, err := client.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp).Msg("")
	//}

	// NFS
	//resp, err := controllerClient.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
	//	Name: "testvol1",
	//	CapacityRange: &csi.CapacityRange{
	//		RequiredBytes: 2 * giB,
	//	},
	//	VolumeCapabilities: []*csi.VolumeCapability{
	//		{
	//			AccessType: &csi.VolumeCapability_Mount{
	//				Mount: &csi.VolumeCapability_MountVolume{},
	//			},
	//			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
	//		},
	//	},
	//})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp).Msg("")
	//}

	// iSCSI
	//resp, err := controllerClient.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
	//	Name: "testvol2",
	//	CapacityRange: &csi.CapacityRange{
	//		RequiredBytes: 2 * giB,
	//	},
	//	VolumeCapabilities: []*csi.VolumeCapability{
	//		{
	//			AccessType: &csi.VolumeCapability_Mount{
	//				Mount: &csi.VolumeCapability_MountVolume{},
	//			},
	//			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
	//		},
	//	},
	//})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp).Msg("")
	//}

	// iSCSI
	//resp2, err := controllerClient.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
	//	VolumeId: "iscsi-testvol1",
	//})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp2).Msg("")
	//}

	// iSCSI
	_, err = controllerClient.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		klog.ErrorS(err, "caught error listing volumes")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// NFS
	//resp2, err := client.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
	//	VolumeId: resp.Volume.GetVolumeId(),
	//})
	//if err != nil {
	//	log.Fatal().Err(err).Msg("")
	//} else {
	//	log.Info().Interface("resp", resp2).Msg("")
	//}

	//nodeClient := csi.NewNodeClient(conn)
	//
	//_, err = nodeClient.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
	//	VolumeId:   "nfs-testvol1",
	//	TargetPath: "/tmp/lala",
	//	VolumeCapability: &csi.VolumeCapability{
	//		AccessType: &csi.VolumeCapability_Mount{
	//			Mount: &csi.VolumeCapability_MountVolume{},
	//		},
	//		AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
	//	},
	//	Readonly:      false,
	//	VolumeContext: resp.Volume.VolumeContext,
	//})
	//if err != nil {
	//	log.Error().Err(err).Msg("")
	//}
	//_, err = nodeClient.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
	//	VolumeId:   "nfs-testvol1",
	//	TargetPath: "/tmp/lala",
	//})
	//if err != nil {
	//	log.Error().Err(err).Msg("")
	//}
}
