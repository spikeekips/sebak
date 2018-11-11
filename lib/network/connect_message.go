package network

import (
	"encoding/json"
	"time"

	"github.com/btcsuite/btcutil/base58"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/common/keypair"
	"boscoin.io/sebak/lib/errors"
	"boscoin.io/sebak/lib/node"
)

type ConnectMessage struct {
	H ConnectMessageHeader
	B ConnectMessageBody
}

type ConnectMessageHeader struct {
	Hash      string `json:"-"`
	Signature string `json:"signature"`
}

type ConnectMessageBody struct {
	Created    string            `json:"created"`
	Address    string            `json:"address"`  // LocalNode.Address()
	Endpoint   *common.Endpoint  `json:"endpoint"` // LocalNode.publishEndpoint()
	Validators []*node.Validator `json:"validators"`
}

func (cb ConnectMessageBody) MakeHashString() string {
	return base58.Encode(common.MustMakeObjectHash(cb))
}

func NewConnectMessage(localNode *node.LocalNode, validators ...*node.Validator) (cm ConnectMessage, err error) {
	// `PublishEndpoint()` must be not empty
	if localNode.PublishEndpoint() == nil {
		err = errors.EndpointNotFound
		return
	}

	cm = ConnectMessage{
		H: ConnectMessageHeader{},
		B: ConnectMessageBody{
			Endpoint:   localNode.PublishEndpoint(),
			Validators: validators,
		},
	}

	return
}

func NewConnectMessageFromJSON(b []byte) (cm ConnectMessage, err error) {
	err = common.DecodeJSONValue(b, &cm)
	return
}

func (cm ConnectMessage) GetHash() string {
	return cm.H.Hash
}

// TODO change to IsWellFormed(config common.Config)

func (cm ConnectMessage) IsWellFormed(config common.Config) error {
	if len(cm.H.Signature) < 1 {
		return errors.InvalidMessage
	}
	if len(cm.B.Created) < 1 {
		return errors.InvalidMessage
	}
	if len(cm.B.Address) < 1 {
		return errors.InvalidMessage
	}
	if len(cm.B.Endpoint.String()) < 1 {
		return errors.InvalidMessage
	}

	// check time
	created, err := common.ParseISO8601(cm.B.Created)
	if err != nil {
		return err
	}

	if err := CheckConnectMessageCreatedTime(created, time.Now()); err != nil {
		return err
	}

	return cm.Verify(config.NetworkID)
}

func (cm ConnectMessage) GetType() common.MessageType {
	return common.ConnectMessage
}

func (cm *ConnectMessage) Sign(kp keypair.KP, networkID []byte) {
	cm.B.Created = common.NowISO8601()
	cm.B.Address = kp.Address()

	cm.H.Hash = cm.B.MakeHashString()
	signature, _ := keypair.MakeSignature(kp, networkID, cm.H.Hash)

	cm.H.Signature = base58.Encode(signature)

	return
}

func (cm ConnectMessage) Verify(networkID []byte) (err error) {
	var kp keypair.KP
	if kp, err = keypair.Parse(cm.B.Address); err != nil {
		return
	}

	hash := cm.B.MakeHashString()

	err = kp.Verify(
		append(networkID, []byte(hash)...),
		base58.Decode(cm.H.Signature),
	)

	return
}

func (cm ConnectMessage) Serialize() (encoded []byte, err error) {
	return json.Marshal(cm)
}

func (cm ConnectMessage) String() string {
	encoded, _ := json.MarshalIndent(cm, "", "  ")
	return string(encoded)
}

// Unconnected returns,
//  * not yet registered validators
//  * registered, but endpoint is changed.
// Unconnected can get the `node.LocalNode.GetValidators()` directly.
func (cm ConnectMessage) Unconnected(validators map[string]*node.Validator) []*node.Validator {
	var unconnected []*node.Validator

	for _, v := range cm.B.Validators {
		if rv, ok := validators[v.Address()]; ok {
			if rv.Endpoint().Equal(v.Endpoint()) {
				continue
			}
		}
		unconnected = append(unconnected, v)
	}

	return unconnected
}

func CheckConnectMessageCreatedTime(a, b time.Time) error {
	sub := a.Sub(b)
	if sub < 0 && sub < (common.ConnectMessageCreatedAllowDuration*-1) {
		return errors.MessageHasIncorrectTime
	}

	if sub > 0 && sub > common.ConnectMessageCreatedAllowDuration {
		return errors.MessageHasIncorrectTime
	}

	return nil
}
