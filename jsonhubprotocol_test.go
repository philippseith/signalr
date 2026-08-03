package signalr

import (
	"bytes"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JSONHubProtocol", func() {
	Context("ParseMessages", func() {
		It("should reject a frame buffer that exceeds maximumReceiveMessageSize", func() {
			p := jsonHubProtocol{}
			p.setDebugLogger(testLogger())
			p.setMaxReceiveMessageSize(100)
			reader, writer := io.Pipe()
			var remainBuf bytes.Buffer
			go func() {
				defer GinkgoRecover()
				// Write 200 bytes with no 0x1e delimiter — forces the buffer past the 100-byte limit
				_, err := writer.Write(bytes.Repeat([]byte("x"), 200))
				Expect(err).NotTo(HaveOccurred())
				_ = writer.Close()
			}()
			_, err := p.ParseMessages(reader, &remainBuf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum receive message size"))
		})
	})
})
