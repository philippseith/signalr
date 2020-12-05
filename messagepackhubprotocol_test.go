package signalr

import (
	"encoding/json"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MessagePackHubProtocol", func() {
	m := messagePackHubProtocol{}
	Context("UnmarshalArgument for numeric types", func() {
		for _, src := range []interface{}{
			float32(1), float64(2),
			int8(3), int16(4), int32(5), int64(55), 6,
			uint8(31), uint16(42), uint32(53), uint64(77), uint(65),
		} {
			src := src // Pin iterator for closure
			data, err := json.Marshal(src)
			It("should be a correct test setup", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("unmarshal correctly to float32", func() {
				var dst float32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to float64", func() {
				var dst float64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int8", func() {
				var dst int8
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int16", func() {
				var dst int16
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int32", func() {
				var dst int32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int64", func() {
				var dst int64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int", func() {
				var dst int
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint8", func() {
				var dst uint8
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint16", func() {
				var dst uint16
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint32", func() {
				var dst uint32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint64", func() {
				var dst uint64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint", func() {
				var dst uint
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
		}
	})
	Context("UnmarshalArgument for signed numeric slice types", func() {
		for _, src := range []interface{}{
			[]float32{1, -2}, []float64{-3, 4},
			[]int8{5, -6, 7}, []int16{14, -15, 16}, []int32{-51, 52, 53},
			[]int64{115, -116}, []int{67, 68},
		} {
			src := src // Pin iterator for closure
			data, err := json.Marshal(src)
			It("should be a correct test setup", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("unmarshal correctly to float32", func() {
				var dst []float32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to float64", func() {
				var dst []float64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int8", func() {
				var dst []int8
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int16", func() {
				var dst []int16
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int32", func() {
				var dst []int32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int64", func() {
				var dst []int64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int", func() {
				var dst []int
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
		}
	})
	Context("UnmarshalArgument for numeric slice types", func() {
		for _, src := range []interface{}{
			[]float32{1, 2}, []float64{3, 4},
			[]int8{5, 6, 7}, []int16{14, 15, 16}, []int32{51, 52, 53},
			[]int64{115, 116}, []int{67, 68},
			[]uint8{31}, []uint16{42, 42, 42}, []uint32{53, 0, 1},
			[]uint64{77, 78}, []uint{65},
		} {
			src := src // Pin iterator for closure
			data, err := json.Marshal(src)
			// Strange: []uint8{31} gets marshaled to "Hw==" But this is not our code, so lets hack
			if _, ok := src.([]uint8); ok {
				data = []byte("[31]")
			}
			It("should be a correct test setup", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("unmarshal correctly to float32", func() {
				var dst []float32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to float64", func() {
				var dst []float64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int8", func() {
				var dst []int8
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int16", func() {
				var dst []int16
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int32", func() {
				var dst []int32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int64", func() {
				var dst []int64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int", func() {
				var dst []int
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint8", func() {
				var dst []uint8
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint16", func() {
				var dst []uint16
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint32", func() {
				var dst []uint32
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint64", func() {
				var dst []uint64
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint", func() {
				var dst []uint
				err := protocol.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
		}
	})
})
