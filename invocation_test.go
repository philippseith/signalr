package signalr

import (
	. "github.com/onsi/ginkgo"
)

type invocationHub struct {
	Hub
}

func (i *invocationHub) Simple() {

}

var _ = Describe("Invocation", func() {

	Describe("Simple invocation", func(){
		Context("When invoked by the client", func() {
			It("should be invoked", func(){

			})
		})
	})

})
