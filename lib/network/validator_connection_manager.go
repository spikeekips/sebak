package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	logging "github.com/inconshreveable/log15"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/errors"
	"boscoin.io/sebak/lib/node"
	"boscoin.io/sebak/lib/voting"
)

type ValidatorConnectionManager struct {
	sync.RWMutex

	localNode *node.LocalNode
	network   Network
	policy    voting.ThresholdPolicy

	validators         map[ /* node.Address() */ string]bool
	clients            map[ /* node.Address() */ string]NetworkClient
	connected          map[ /* node.Address() */ string]bool
	cm                 ConnectMessage
	receiveConnectChan chan ConnectMessage
	config             common.Config

	log     logging.Logger
	isReady bool
}

func NewValidatorConnectionManager(
	localNode *node.LocalNode,
	network Network,
	policy voting.ThresholdPolicy,
	config common.Config,
) ConnectionManager {
	if len(localNode.GetValidators()) == 0 {
		panic("empty validators")
	}
	validators := map[string]bool{
		localNode.Address(): true,
	}

	var cm ConnectMessage
	var err error
	if cm, err = NewConnectMessage(localNode, localNode.ConvertToValidator()); err != nil {
		panic(err)
	}

	c := &ValidatorConnectionManager{
		localNode:  localNode,
		network:    network,
		policy:     policy,
		validators: validators,

		clients:            map[string]NetworkClient{},
		connected:          map[string]bool{},
		receiveConnectChan: make(chan ConnectMessage, 100),
		config:             config,
		log:                log.New(logging.Ctx{"node": localNode.Alias()}),
		cm:                 cm,
	}
	c.connected[localNode.Address()] = true

	return c
}

func (c *ValidatorConnectionManager) Start() {
	c.log.Debug(
		"starting to connect to validators",
		"validators", c.localNode.GetValidators(),
	)

	logCurrentState := func() {
		c.log.Debug(
			"current validators",
			"validators", logging.Lazy{func() []string {
				var vs []string
				for _, v := range c.localNode.GetValidators() {
					vs = append(vs, v.Address())
				}
				return vs
			}},
			"alive", logging.Lazy{c.AllConnected},
			"connected", logging.Lazy{func() []string {
				var vs []string
				for v, _ := range c.validators {
					vs = append(vs, v)
				}
				return vs
			}},
		)
	}

	go func() {
		ticker := time.NewTicker(time.Second * 1)
		for _ = range ticker.C {
			logCurrentState()
		}
	}()

	c.startConnect()
	go c.startAlive()

	ticker := time.NewTicker(time.Second * 1)
	for _ = range ticker.C {
		if c.CountConnected() >= c.policy.Threshold() {
			ticker.Stop()
			break
		}
	}

	c.isReady = true

	logCurrentState()
}

func (c *ValidatorConnectionManager) GetNodeAddress() string {
	return c.localNode.Address()
}

func (c *ValidatorConnectionManager) getConnectionByEndpoint(endpoint *common.Endpoint) (client NetworkClient) {
	if endpoint == nil {
		return nil
	}

	c.RLock()
	var ok bool
	client, ok = c.clients[endpoint.String()]
	if ok {
		c.RUnlock()
		return
	}
	c.RUnlock()

	client = c.network.GetClient(endpoint)
	if client == nil {
		return nil
	}

	c.Lock()
	defer c.Unlock()

	c.clients[endpoint.String()] = client

	return
}

func (c *ValidatorConnectionManager) GetConnection(address string) (client NetworkClient) {
	if v := c.localNode.Validator(address); v == nil {
		return nil
	} else if v.Endpoint() == nil {
		return nil
	} else {
		return c.getConnectionByEndpoint(v.Endpoint())
	}
}

func (c *ValidatorConnectionManager) startAlive() {
	c.log.Debug("starting to check alive of validators", "validators", c.getValidators())

	check := func() {
		for address, _ := range c.getValidators() {
			if address == c.localNode.Address() {
				continue
			}
			go c.checkingAlive(address)
		}
	}

	check()

	ticker := time.NewTicker(time.Second * 5)
	for _ = range ticker.C {
		check()
	}
}

// setConnected returns `true` when the validator is newly connected or
// disconnected at first
func (c *ValidatorConnectionManager) setConnected(address string, connected bool) bool {
	c.Lock()
	defer c.Unlock()

	old, found := c.connected[address]
	c.connected[address] = connected

	c.policy.SetConnected(c.countConnectedUnlocked())
	return !found || old != connected
}

func (c *ValidatorConnectionManager) AllConnected() []string {
	c.RLock()
	defer c.RUnlock()
	var connected []string
	for address, isConnected := range c.connected {
		if !isConnected {
			continue
		}
		connected = append(connected, address)
	}

	return connected
}

// Returns:
//   A list of all validators
func (c *ValidatorConnectionManager) AllValidators() []string {
	var validators []string
	for address, _ := range c.localNode.GetValidators() {
		validators = append(validators, address)
	}
	return validators
}

//
// Returns:
//   the number of validators which are currently connected
//
func (c *ValidatorConnectionManager) CountConnected() int {
	c.RLock()
	defer c.RUnlock()
	return c.countConnectedUnlocked()
}

func (c *ValidatorConnectionManager) countConnectedUnlocked() int {
	var count int
	for _, isConnected := range c.connected {
		if isConnected {
			count += 1
		}
	}
	return count
}

func (c *ValidatorConnectionManager) checkingAlive(address string) {
	client := c.GetConnection(address)
	if client == nil {
		c.log.Error("failed to get client", "validator", address)
		return
	}

	err := client.Alive()
	if c.setConnected(address, err == nil) {
		if err == nil {
			c.log.Debug("validator is connected", "validator", address)
		} else {
			c.log.Debug("validator is disconnected", "validator", address, "error", err)
		}
	}

	return
}

func (c *ValidatorConnectionManager) ConnectionWatcher(t Network, conn net.Conn, state http.ConnState) {
	return
}

func (c *ValidatorConnectionManager) Broadcast(message common.Message) {
	c.RLock()
	defer c.RUnlock()

	for address, connected := range c.connected {
		if !connected {
			continue
		}

		go func(address string) {
			client := c.GetConnection(address)
			if client == nil {
				c.log.Error("failed to get client", "validator", address)
				return
			}

			var err error
			var response []byte
			if message.GetType() == common.BallotMessage {
				response, err = client.SendBallot(message)
			} else if message.GetType() == common.TransactionMessage {
				response, err = client.SendMessage(message)
			} else {
				panic("invalid message")
			}

			if err != nil {
				c.log.Error(
					"failed to broadcast",
					"error", err,
					"validator", address,
					"type", message.GetType(),
					"message", message.GetHash(),
					"response", string(response),
				)
			}
		}(address)
	}
	return
}

func (c *ValidatorConnectionManager) GetNode(address string) node.Node {
	c.RLock()
	defer c.RUnlock()

	if !c.localNode.HasValidator(address) {
		return nil
	}

	return c.localNode.Validator(address)
}

// startConnect blocks until connected validators over threshold.
func (c *ValidatorConnectionManager) startConnect() {
	c.log.Debug("starting to get to validators")

	ctx, done := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case cm := <-c.receiveConnectChan:
				err := c.receivedConnect(done, cm)
				if err != nil {
					c.log.Error("failed to get ConnectMessage", "error", err)
				}
			}
		}
	}()

	if len(c.AllValidators()) == 1 && c.AllValidators()[0] == c.localNode.Address() {
		c.log.Debug("validators except LocalNode is empty")
		done()
		goto end
	}

	for {
		select {
		case <-ctx.Done():
			goto end
		default:
			var knowns []string
			for _, v := range c.localNode.GetValidators() {
				if v.Address() == c.localNode.Address() {
					continue
				}

				if v.Endpoint() == nil {
					continue
				}

				knowns = append(knowns, v.Address())
			}

			for _, address := range knowns {
				go c.sendConnectMessage(c.cm, address)
			}

			time.Sleep(time.Second * 1)
		}
	}

end:
	c.log.Debug(
		"connected over threshold",
		"validators", len(c.getValidators()),
		"threshold", c.policy.Threshold(),
	)
}

func (c *ValidatorConnectionManager) sendConnectMessage(cm ConnectMessage, address string) (err error) {
	connected := c.localNode.Validator(address)
	if connected == nil {
		err = fmt.Errorf("unknown validator")
		c.log.Error(err.Error(), "validator", address)
		return
	}

	endpoint := connected.Endpoint()
	if endpoint == nil {
		return
	}

	client := c.getConnectionByEndpoint(endpoint)
	if client == nil {
		err = fmt.Errorf("failed to get client")
		return
	}

	cm.Sign(c.localNode.Keypair(), c.config.NetworkID)

	var b []byte
	if b, err = client.Connect(cm); err != nil {
		c.log.Error("failed to send connect message", "endpoint", endpoint, "error", err)
		return
	} else if len(b) < 1 {
		c.log.Error("got empty response", "endpoint", endpoint, "error", err)
		err = errors.InvalidMessage
		return
	}

	var received ConnectMessage
	if received, err = NewConnectMessageFromJSON(b); err != nil {
		c.log.Error("got weired ConnectMessage", "error", err, "message", string(b))
		return
	}

	if err = received.IsWellFormed(common.Config{}); err != nil {
		c.log.Error("failed to  ConnectMessage.IsWellFormed()", "error", err, "cm", received)
		return
	}

	c.ReceiveConnect(received)

	return
}

func (c *ValidatorConnectionManager) ReceiveConnect(cm ConnectMessage) {
	c.receiveConnectChan <- cm
}

func (c *ValidatorConnectionManager) getValidators() map[string]bool {
	c.RLock()
	defer c.RUnlock()

	return c.validators
}

func (c *ValidatorConnectionManager) ConnectedValidators() map[string]*node.Validator {
	c.RLock()
	defer c.RUnlock()

	vs := map[string]*node.Validator{}
	for address, _ := range c.getValidators() {
		connected := c.localNode.Validator(address)
		if connected == nil {
			continue
		}
		vs[address] = connected
	}

	return vs
}

func (c *ValidatorConnectionManager) setConnectedValidators(vs ...*node.Validator) {
	c.Lock()
	defer c.Unlock()

	for _, v := range vs {
		connected := c.localNode.Validator(v.Address())
		if connected == nil {
			continue
		}

		c.validators[v.Address()] = true
		connected.SetEndpoint(v.Endpoint())
	}
}

func (c *ValidatorConnectionManager) receivedConnect(done context.CancelFunc, cm ConnectMessage) (err error) {
	unconnected := cm.Unconnected(c.ConnectedValidators())
	if len(unconnected) < 1 {
		c.log.Debug(
			"new validators not found",
			"from", cm.B.Address,
			"received", cm.B.Validators,
			"current", c.getValidators(),
			"cm", cm,
		)
		return
	}

	c.setConnectedValidators(unconnected...)

	c.log.Debug(
		"new validators found",
		"from", cm.B.Address,
		"received", cm.B.Validators,
		"new", unconnected,
		"after", c.getValidators(),
	)

	// broadcast new validators to all validators.
	for address, _ := range c.getValidators() {
		if address == c.localNode.Address() {
			continue
		}

		go c.sendConnectMessage(c.cm, address)
	}

	if len(c.getValidators()) >= c.policy.Threshold() {
		done()
	}

	return
}

func (c *ValidatorConnectionManager) IsReady() bool {
	return c.isReady
}
