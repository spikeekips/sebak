package network

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/common/keypair"
	"boscoin.io/sebak/lib/errors"
	"boscoin.io/sebak/lib/node"
)

func TestConnectMessage(t *testing.T) {
	var networkID []byte = []byte("show-me")

	kp := keypair.Random()
	endpoint, _ := common.NewEndpointFromString("http://1.2.3.4:5678")
	localNode, _ := node.NewLocalNode(kp, endpoint, "")

	var validators []*node.Validator
	{ // add validators
		for i := 0; i < 3; i++ {
			kpv := keypair.Random()
			endpointv, _ := common.NewEndpointFromString(fmt.Sprintf("http://1.2.3.4:567%d", i))
			v, _ := node.NewValidator(kpv.Address(), endpointv, "")
			validators = append(validators, v)
		}
	}
	localNode.AddValidators(validators...)

	{ // localNode.PublishEndpoint() is empty
		localNode.SetPublishEndpoint(nil)
		_, err := NewConnectMessage(localNode)
		require.Error(t, errors.EndpointNotFound, err)
	}

	{ // localNode.PublishEndpoint() is not empty
		localNode.SetPublishEndpoint(endpoint)

		cm, err := NewConnectMessage(localNode, validators...)
		require.NoError(t, err)

		require.Equal(t, len(validators), len(cm.B.Validators))

		// Before signing, `Created` must be empty
		require.Empty(t, cm.B.Created)
	}

	{ // signing
		cm, _ := NewConnectMessage(localNode)
		cm.Sign(localNode.Keypair(), networkID)

		require.NotEmpty(t, cm.H.Signature)
		require.NotEmpty(t, cm.B.Created)
		require.NotEmpty(t, cm.B.Address)
		require.NotEmpty(t, cm.B.Endpoint)

		for _, v := range cm.B.Validators {
			require.NotEmpty(t, v.Address())
			require.NotEmpty(t, v.Alias())
			require.NotEmpty(t, v.Endpoint())
		}
	}

	{ //  and verification
		cm, _ := NewConnectMessage(localNode)
		cm.Sign(localNode.Keypair(), networkID)
		err := cm.Verify(networkID)
		require.NoError(t, err)
	}

	{ // from json; verification
		cm, _ := NewConnectMessage(localNode)
		cm.Sign(localNode.Keypair(), networkID)

		b, err := cm.Serialize()
		require.NoError(t, err)

		cmJson, err := NewConnectMessageFromJSON(b)
		require.NoError(t, err)

		err = cmJson.Verify(networkID)
		require.NoError(t, err)
	}
}

func TestConnectMessageUnconnected(t *testing.T) {
	var networkID []byte = []byte("show-me")

	kp := keypair.Random()
	endpoint, _ := common.NewEndpointFromString("http://1.2.3.4:5678")
	localNode, _ := node.NewLocalNode(kp, endpoint, "")

	var validators []*node.Validator
	mapValidators := map[string]*node.Validator{}
	{ // add validators
		for i := 0; i < 4; i++ {
			kpv := keypair.Random()
			endpointv, _ := common.NewEndpointFromString(fmt.Sprintf("http://1.2.3.4:567%d", i))
			v, _ := node.NewValidator(kpv.Address(), endpointv, "")
			validators = append(validators, v)
		}
	}
	localNode.AddValidators(validators[:3]...)
	localNode.SetPublishEndpoint(endpoint)

	cm, _ := NewConnectMessage(localNode, validators...)
	cm.Sign(localNode.Keypair(), networkID)

	{ // unregistered validator must be returned
		unconnected := cm.Unconnected(mapValidators)
		require.Equal(t, len(cm.B.Validators), len(unconnected))
		require.NotNil(t, unconnected)
	}

	{ // endpoint not updated validators must not be returned
		validator := cm.B.Validators[0]
		mapValidators[validator.Address()] = validator

		unconnected := cm.Unconnected(mapValidators)
		require.Equal(t, len(cm.B.Validators)-1, len(unconnected))
		require.NotNil(t, unconnected)
	}

	{ // endpoint updated validators must be returned
		newEndpoint, _ := common.NewEndpointFromString("http://4.3.2.1")
		validator, _ := node.NewValidator(cm.B.Validators[0].Address(), newEndpoint, "")
		mapValidators[validator.Address()] = validator

		unconnected := cm.Unconnected(mapValidators)
		require.Equal(t, len(cm.B.Validators), len(unconnected))
		require.NotNil(t, unconnected)
	}
}
