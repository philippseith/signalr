package signalr_test

import (
	. "github.com/onsi/ginkgo"
	"github.com/philippseith/signalr"
)

type invocationHub struct {
	signalr.Hub
}

func (i *invocationHub) Simple() {

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
