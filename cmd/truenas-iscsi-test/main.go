package main

import (
	"context"
	"os"

	"k8s.io/klog/v2"

	tnclient "github.com/terrycain/truenas-go-sdk"
	"golang.org/x/oauth2"
)

func main() {
	accessToken := os.Getenv("TRUENAS_TOKEN")
	url := os.Getenv("TRUENAS_URL")
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(context.Background(), ts)
	config := tnclient.NewConfiguration()
	config.Servers = tnclient.ServerConfigurations{{URL: url}}
	config.Debug = true
	config.HTTPClient = tc
	client := tnclient.NewAPIClient(config)
	ctx := context.Background()

	// Get global configuration
	// resp, _, err := client.IscsiGlobalApi.GetISCSIGlobalConfiguration(ctx).Execute()

	// List portals
	// resp, _, err := client.IscsiPortalApi.ListISCSIPortal(ctx).Execute()

	// Get portal
	resp, _, err := client.IscsiPortalApi.GetISCSIPortal(ctx, 1).Execute()

	// Create iSCSI Extent
	//param := tnclient.CreateISCSIExtentParams{
	//	Name: "test1234",
	//	Rpm: tnclient.PtrString("SSD"),
	//	Type: "DISK",
	//	InsecureTpc: tnclient.PtrBool(true),
	//	Xen: tnclient.PtrBool(false),
	//    Comment: tnclient.PtrString("truenas-csi: blah"),
	//	Blocksize: tnclient.PtrInt32(512),
	//	Disk: *tnclient.NewNullableString(tnclient.PtrString("zvol/" + "kube/csi/test1234")),
	//}
	//resp, _, err := client.IscsiExtentApi.CreateISCSIExtent(ctx).CreateISCSIExtentParams(param).Execute()

	// List iSCSI Extents
	// resp, _, err := client.IscsiExtentApi.ListISCSIExtent(ctx).Execute()

	// Get iSCSI Extent
	// resp, _, err := client.IscsiExtentApi.GetISCSIExtent(ctx, 2).Execute()

	// Delete iSCSI Extent
	//params := tnclient.DeleteISCSIExtentParams{
	//	Remove: true,
	//	Force: true,
	//}
	//resp, err := client.IscsiExtentApi.DeleteISCSIExtent(ctx, 2).DeleteISCSIExtentParams(params).Execute()

	// Create iSCSI Initiator
	//param := tnclient.CreateISCSIInitiatorParams{
	//	Comment: tnclient.PtrString("truenas-csi: blah"),
	//}
	//resp, _, err := client.IscsiInitiatorApi.CreateISCSIInitiator(ctx).CreateISCSIInitiatorParams(param).Execute()

	// List iSCSI Initiators
	// resp, _, err := client.IscsiInitiatorApi.ListISCSIInitiator(ctx).Execute()

	// Get iSCSI Initiators
	// resp, _, err := client.IscsiInitiatorApi.GetISCSIInitiator(ctx, 1).Execute()

	// Delete iSCSI Initiators
	// resp, err := client.IscsiInitiatorApi.DeleteISCSIInitiator(ctx, 8).Execute()

	// Create iSCSI Target
	//param := tnclient.CreateISCSITargetParams{
	//	Name: "iscsi-blah",
	//	Mode: tnclient.PtrString("ISCSI"),
	//	Groups: []tnclient.CreateISCSITargetParamsGroupsInner{
	//		{
	//			Portal:               1,
	//			Initiator:            tnclient.PtrInt32(1),
	//			Authmethod: "NONE",
	//		},
	//	},
	//
	//}
	//resp, _, err := client.IscsiTargetApi.CreateISCSITarget(ctx).CreateISCSITargetParams(param).Execute()

	// List iSCSI Targets
	// resp, _, err := client.IscsiTargetApi.ListISCSITarget(ctx).Execute()

	// Get iSCSI Target
	// resp, _, err := client.IscsiTargetApi.GetISCSITarget(ctx, 2).Execute()

	// Delete iSCSI Target (deletes extent mapping)
	// resp, err := client.IscsiTargetApi.DeleteISCSITarget(ctx, 2).Body(true).Execute()

	// Create iSCSI Target Extent mapping
	//param := tnclient.CreateISCSITargetExtentParams{
	//	Target: 2,
	//	Extent: 2,
	//}
	//resp, _, err := client.IscsiTargetextentApi.CreateISCSITargetExtent(ctx).CreateISCSITargetExtentParams(param).Execute()

	// List iSCSI Target Extent mappings
	// resp, _, err := client.IscsiTargetextentApi.ListISCSITargetExtent(ctx).Execute()

	// Get iSCSI Target Extent mapping
	// resp, _, err := client.IscsiTargetextentApi.GetISCSITargetExtent(ctx, 6).Execute()

	// Delete iSCSI Target Extent mapping
	// resp, err := client.IscsiTargetextentApi.DeleteISCSITargetExtent(ctx, 6).Body(true).Execute()

	klog.InfoS("sent", "response", resp)
	if err != nil {
		klog.ErrorS(err, "failed to send request")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}
