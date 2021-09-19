package signalr

import (
	"bytes"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MessagePackHubProtocol", func() {
	protocol := messagePackHubProtocol{}
	protocol.setDebugLogger(testLogger())
	Context("ParseMessages", func() {
		It("should encode/decode an InvocationMessage", func() {
			message := invocationMessage{
				Type:         4,
				Target:       "target",
				InvocationID: "1",
				// because DecodeSlice below decodes ints to the smallest type and arrays always to []interface{}, we need to be very specific
				Arguments: []interface{}{"1", int8(1), []interface{}{int8(7), int8(3)}},
				StreamIds: []string{"0"},
			}
			buf := bytes.Buffer{}
			err := protocol.WriteMessage(message, &buf)
			Expect(err).NotTo(HaveOccurred())
			remainBuf := bytes.Buffer{}
			got, err := protocol.ParseMessages(&buf, &remainBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(remainBuf.Len()).To(Equal(0))
			Expect(len(got)).To(Equal(1))
			Expect(got[0]).To(BeAssignableToTypeOf(invocationMessage{}))
			gotMsg := got[0].(invocationMessage)
			Expect(gotMsg.Type).To(Equal(message.Type))
			Expect(gotMsg.Target).To(Equal(message.Target))
			Expect(gotMsg.InvocationID).To(Equal(message.InvocationID))
			Expect(gotMsg.StreamIds).To(Equal(message.StreamIds))
			for i, gotArg := range gotMsg.Arguments {
				// We can not directly compare gotArg and want.Arguments[i]
				// because msgpack serializes numbers to the shortest possible type
				t := reflect.TypeOf(message.Arguments[i])
				value := reflect.New(t)
				Expect(protocol.UnmarshalArgument(gotArg, value.Interface())).NotTo(HaveOccurred())
				Expect(reflect.Indirect(value).Interface()).To(Equal(message.Arguments[i]))
			}
		})
	})
})
