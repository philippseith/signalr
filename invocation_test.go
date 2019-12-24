package signalr_test

import (
	"github.com/golang-collections/go-datastructures/queue"
	. "github.com/onsi/ginkgo"
	"github.com/philippseith/signalr"
)

var invocationQueue = queue.New(10)

type invocationHub struct {
	signalr.Hub
}

func (i *invocationHub) Simple() {
	invocationQueue.Put("Simple()")
}

var _ = Describe("Invocation", func() {

	server := signalr.NewServer(&invocationHub{})
	go server.MessageLoop(newTestingHubConnection(), "bla", &signalr.JsonHubProtocol{})
	Describe("Simple invocation", func(){
		Context("When invoked by the client", func() {
			It("should be invoked", func(){
				
			})
		})
	})

})
