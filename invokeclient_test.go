package signalr

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("Invocation", func() {

	Describe("", func() {
		It("s", func() {
			ic := newInvokeClient(time.Second * 2)
			Expect(ic).NotTo(BeNil())

		})
	})
})
