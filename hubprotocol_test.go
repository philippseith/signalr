package signalr

import (
	"bytes"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/mailru/easyjson/jwriter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
)

var _ = FDescribe("Protocol", func() {
	for _, p := range []HubProtocol{
		&JSONHubProtocol{easyWriter: jwriter.Writer{}},
		//&MessagePackHubProtocol{},
	} {
		p.setDebugLogger(log.NewLogfmtLogger(os.Stderr))
		var buf bytes.Buffer
		Describe(fmt.Sprintf("%T: WriteMessage/ParseMessage roundtrip", p), func() {
			Context("InvocationMessage", func() {
				It("be equal after roundtrip", func() {
					want := invocationMessage{
						Type:         1,
						Target:       "",
						InvocationID: "",
						Arguments:    make([]interface{}, 0),
						StreamIds:    nil,
					}
					Expect(p.WriteMessage(want, &buf)).NotTo(HaveOccurred())
					got, err := p.ParseMessage(&buf)
					Expect(err).NotTo(HaveOccurred())
					Expect(got).To(Equal(want))
				})
			})
		})
	}
})
