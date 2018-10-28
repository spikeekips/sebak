package runner

import (
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/node"
	"boscoin.io/sebak/lib/version"
)

func NewNodeInfo(nr *NodeRunner) node.NodeInfo {
	localNode := nr.Node()

	var endpoint *common.Endpoint
	if localNode.PublishEndpoint() != nil {
		endpoint = localNode.PublishEndpoint()
	}

	nv := node.NodeVersion{
		Version:   version.Version,
		GitCommit: version.GitCommit,
		GitState:  version.GitState,
		BuildDate: version.BuildDate,
	}

	nd := node.NodeInfoNode{
		Version:    nv,
		State:      localNode.State(),
		Alias:      localNode.Alias(),
		Address:    localNode.Address(),
		Endpoint:   endpoint,
		Validators: localNode.GetValidators(),
	}

	policy := node.NodePolicy{
		NetworkID:                 string(nr.NetworkID()),
		InitialBalance:            nr.InitialBalance,
		BaseReserve:               common.BaseReserve,
		BaseFee:                   common.BaseFee,
		BlockTime:                 nr.Conf.BlockTime,
		OperationsLimit:           nr.Conf.OpsLimit,
		TransactionsLimit:         nr.Conf.TxsLimit,
		GenesisBlockConfirmedTime: common.GenesisBlockConfirmedTime,
		InflationRatio:            common.InflationRatioString,
		BlockHeightEndOfInflation: common.BlockHeightEndOfInflation,
	}

	return node.NodeInfo{
		Node:   nd,
		Policy: policy,
	}
}
