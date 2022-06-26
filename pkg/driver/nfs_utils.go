package driver

import (
	"context"
	tnclient "github.com/terrycain/truenas-go-sdk"
)

type DatasetMatcher func(dataset tnclient.Dataset) bool
type NFSShareMatcher func(share tnclient.ShareNFS) bool

func FindDataset(ctx context.Context, client *tnclient.APIClient, fn DatasetMatcher) (tnclient.Dataset, bool, error) {
	datasets, _, err := client.DatasetApi.ListDatasets(ctx).Execute()
	if err != nil {
		return tnclient.Dataset{}, false, err
	}

	for _, dataset := range datasets {
		if fn(dataset) {
			return dataset, true, nil
		}
	}

	return tnclient.Dataset{}, false, nil
}

func FindAllDatasets(ctx context.Context, client *tnclient.APIClient, fn DatasetMatcher) ([]tnclient.Dataset, error) {
	datasets, _, err := client.DatasetApi.ListDatasets(ctx).Execute()
	if err != nil {
		return []tnclient.Dataset{}, err
	}

	result := make([]tnclient.Dataset, 0)

	for _, dataset := range datasets {
		ds := dataset
		if fn(ds) {
			result = append(result, ds)
		}
	}

	return result, nil
}

func FindNFSShare(ctx context.Context, client *tnclient.APIClient, fn NFSShareMatcher) (tnclient.ShareNFS, bool, error) {
	shares, _, err := client.SharingApi.ListSharesNFS(ctx).Execute()
	if err != nil {
		return tnclient.ShareNFS{}, false, err
	}

	for _, share := range shares {
		if fn(share) {
			return share, true, nil
		}
	}

	return tnclient.ShareNFS{}, false, nil
}

func FindAllNFSShares(ctx context.Context, client *tnclient.APIClient, fn NFSShareMatcher) ([]tnclient.ShareNFS, error) {
	shares, _, err := client.SharingApi.ListSharesNFS(ctx).Execute()
	if err != nil {
		return []tnclient.ShareNFS{}, err
	}

	result := make([]tnclient.ShareNFS, 0)

	for _, share := range shares {
		s := share
		if fn(s) {
			result = append(result, s)
		}
	}

	return result, nil
}