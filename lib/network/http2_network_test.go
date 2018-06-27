package sebaknetwork

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"unicode"

	"net"
	"time"

	"boscoin.io/sebak/lib/common"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
)

var testPort int = 5000
var once sync.Once

func getPort() int {
	once.Do(func() {
		testPort = sebakcommon.GetFreePort()
	})

	return testPort
}

func ExampleHttp2NetworkConfigCreateWithNonTLS() {

	var config HTTP2NetworkConfig
	endpoint, err := sebakcommon.NewEndpointFromString("https://localhost:5000?NodeName=n1")
	if err != nil {
		fmt.Print("Error in NewEndpointFromString")
	}
	queries := endpoint.Query()
	queries.Add("TLSCertFile", "")
	queries.Add("TLSKeyFile", "")
	endpoint.RawQuery = queries.Encode()

	config, err = NewHTTP2NetworkConfigFromEndpoint(endpoint)
	if err != nil {
		fmt.Print("Error in NewHTTP2NetworkConfigFromEndpoint")
	}
	fmt.Println(config.NodeName)
	fmt.Println(config.Addr)

	// Output: n1
	// localhost:5000
}

const (
	dirPath  = "tmp"
	certPath = "cert.pem"
	keyPath  = "key.pem"
)

func createNewHTTP2Network() (
	kp *keypair.Full,
	mn *HTTP2Network,
	validator *sebakcommon.Validator,
	startFunc func(),
	err error,
) {
	g := NewKeyGenerator(dirPath, certPath, keyPath)

	var config HTTP2NetworkConfig
	host := fmt.Sprintf("localhost:%d", getPort())

	var endpoint *sebakcommon.Endpoint
	endpoint, err = sebakcommon.NewEndpointFromString(fmt.Sprintf("https://%s?NodeName=n1", host))
	if err != nil {
		return
	}

	queries := endpoint.Query()
	queries.Add("TLSCertFile", g.GetCertPath())
	queries.Add("TLSKeyFile", g.GetKeyPath())
	endpoint.RawQuery = queries.Encode()

	config, err = NewHTTP2NetworkConfigFromEndpoint(endpoint)
	if err != nil {
		return
	}
	mn = NewHTTP2Network(config)

	kp, _ = keypair.Random()
	validator, _ = sebakcommon.NewValidator(kp.Address(), mn.Endpoint(), "")
	validator.SetKeypair(kp)

	mn.SetContext(context.WithValue(context.Background(), "currentNode", validator))

	startFunc = func() {
		mn.Ready()
		go mn.Start()

		time.Sleep(500 * time.Millisecond)

		// check connection availability
		for {
			if _, err := net.DialTimeout("tcp", host, 100*time.Millisecond); err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return
		}
	}

	return
}

type TestMessageBroker struct{}

func (r TestMessageBroker) ResponseMessage(w http.ResponseWriter, o string) {
	fmt.Fprintf(w, o)
}

func (r TestMessageBroker) ReceiveMessage(*HTTP2Network, Message) {}

func removeWhiteSpaces(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, str)
}

func TestHTTP2NetworkGetNodeInfo(t *testing.T) {
	_, s0, currentNode, startFunc, err := createNewHTTP2Network()
	if err != nil {
		t.Error(err)
		return
	}

	s0.SetMessageBroker(TestMessageBroker{})
	startFunc()
	defer s0.Stop()

	c0 := s0.GetClient(s0.Endpoint())

	b, err := c0.GetNodeInfo()
	if err != nil {
		t.Error(err)
		return
	}
	v, err := sebakcommon.NewValidatorFromString(b)
	if err != nil {
		t.Error(err)
		return
	}

	server := currentNode.Endpoint().String()
	client := v.Endpoint().String()

	assert.Equal(t, server, client, "Server endpoint and received endpoint should be the same.")
	assert.Equal(t, currentNode.Address(), v.Address(), "Server address and received address should be the same.")
}

type StringResponseMessageBroker struct {
	msg string
}

func (r StringResponseMessageBroker) ResponseMessage(w http.ResponseWriter, _ string) {
	fmt.Fprintf(w, r.msg)
}

func (r StringResponseMessageBroker) ReceiveMessage(*HTTP2Network, Message) {}

func TestHTTP2NetworkMessageBrokerResponseMessage(t *testing.T) {
	_, s0, currentNode, startFunc, err := createNewHTTP2Network()
	if err != nil {
		t.Error(err)
		return
	}

	s0.SetMessageBroker(StringResponseMessageBroker{"ResponseMessage"})
	startFunc()
	defer s0.Stop()

	c0 := s0.GetClient(s0.Endpoint())

	returnMsg, _ := c0.Connect(currentNode)

	assert.Equal(t, string(returnMsg), "ResponseMessage", "The connectNode and the return should be the same.")
}

func TestHTTP2NetworkConnect(t *testing.T) {
	_, s0, currentNode, startFunc, err := createNewHTTP2Network()
	if err != nil {
		t.Error(err)
		return
	}

	s0.SetMessageBroker(TestMessageBroker{})
	startFunc()
	defer s0.Stop()

	c0 := s0.GetClient(s0.Endpoint())

	o, _ := currentNode.Serialize()
	nodeStr := removeWhiteSpaces(string(o))

	returnMsg, _ := c0.Connect(currentNode)
	returnStr := removeWhiteSpaces(string(returnMsg))

	assert.Equal(t, returnStr, nodeStr, "The connectNode and the return should be the same.")
}

func TestHTTP2NetworkSendMessage(t *testing.T) {
	_, s0, _, startFunc, err := createNewHTTP2Network()
	if err != nil {
		t.Error(err)
		return
	}

	s0.SetMessageBroker(TestMessageBroker{})
	startFunc()
	defer s0.Stop()

	msg := NewDummyMessage("findme")
	c0 := s0.GetClient(s0.Endpoint())
	returnMsg, _ := c0.SendMessage(msg)

	returnStr := removeWhiteSpaces(string(returnMsg))
	sendMsg := removeWhiteSpaces(msg.String())

	assert.Equal(t, returnStr, sendMsg, "The sendMessage and the return should be the same.")
}

func TestHTTP2NetworkSendBallot(t *testing.T) {
	_, s0, _, startFunc, err := createNewHTTP2Network()
	if err != nil {
		t.Error(err)
		return
	}

	s0.SetMessageBroker(TestMessageBroker{})
	startFunc()
	defer s0.Stop()

	c0 := s0.GetClient(s0.Endpoint())

	msg := NewDummyMessage("findme")
	returnMsg, _ := c0.SendBallot(msg)

	returnStr := removeWhiteSpaces(string(returnMsg))
	sendMsg := removeWhiteSpaces(msg.String())

	assert.Equal(t, returnStr, sendMsg, "The sendBallot and the return should be the same.")
}
