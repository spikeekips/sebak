package runner

import (
	"io/ioutil"
	"net/http"

	"boscoin.io/sebak/lib/errors"
	"boscoin.io/sebak/lib/network"
	"boscoin.io/sebak/lib/network/httputils"
	"boscoin.io/sebak/lib/node"
)

// ConnectHandler will receive the `ConnectMessage` and checks the
// unconnected validators. If found, trying to connect.
func (nh NetworkHandlerNode) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	cm, err := network.NewConnectMessageFromJSON(body)
	if err != nil {
		http.Error(w, err.Error(), httputils.StatusCode(err))
		return
	}

	if err := cm.IsWellFormed(nh.conf); err != nil {
		http.Error(w, err.Error(), httputils.StatusCode(err))
		return
	}

	if !nh.localNode.HasValidator(cm.B.Address) {
		err := errors.ConnectFromUnknownValidator
		http.Error(w, err.Error(), httputils.StatusCode(err))
		return
	}

	nh.connectionManager.ReceiveConnect(cm)

	var validators []*node.Validator
	for _, v := range nh.localNode.GetValidators() {
		if v.Endpoint() == nil {
			continue
		}

		validators = append(validators, v)
	}

	responseCm, err := network.NewConnectMessage(nh.localNode, validators...)
	if err != nil {
		http.Error(w, err.Error(), httputils.StatusCode(err))
		return
	}

	responseCm.Sign(nh.localNode.Keypair(), nh.conf.NetworkID)
	b, err := responseCm.Serialize()
	if err != nil {
		http.Error(w, err.Error(), httputils.StatusCode(err))
		return
	}
	nh.network.MessageBroker().Response(w, b)
}
