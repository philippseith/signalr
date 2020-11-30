package signalr

import (
	"encoding/json"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MessagePackHubProtocol", func() {
	m := messagePackHubProtocol{}
	Context("UnmarshalArgument for strings", func() {
		for i, src := range []interface{}{
			"10", "0x1e", "0b00010001", "5.22", "NotANumber"} {
			i := i     // pin iterator for closure
			src := src // pin iterator for closure
			It("unmarshal correctly from string to float32", func() {
				var dst float32
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float32(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float32(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float32(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float32(5.22)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to float64", func() {
				var dst float64
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float64(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float64(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(float64(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(5.22))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to int8", func() {
				var dst int8
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int8(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int8(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int8(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(int8(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to int16", func() {
				var dst int16
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int16(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int16(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int16(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(int16(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to int32", func() {
				var dst int32
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int32(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int32(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int32(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(int32(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to int64", func() {
				var dst int64
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int64(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int64(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(int64(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(int64(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to int", func() {
				var dst int
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(10))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(30))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(17))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(int(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to  uint8", func() {
				var dst uint8
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint8(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint8(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint8(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(uint8(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to  uint16", func() {
				var dst uint16
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint16(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint16(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint16(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(uint16(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to  uint32", func() {
				var dst uint32
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint32(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint32(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint32(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(uint32(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to  uint64", func() {
				var dst uint64
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint64(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint64(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint64(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(uint64(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
			It("unmarshal correctly from string to uint", func() {
				var dst uint
				err := m.UnmarshalArgument(src, &dst)
				switch i {
				case 0:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint(10)))
				case 1:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint(30)))
				case 2:
					Expect(err).NotTo(HaveOccurred())
					Expect(dst).To(Equal(uint(17)))
				case 3:
					Expect(err).NotTo(HaveOccurred())
					f := 5.22
					Expect(dst).To(Equal(uint(f)))
				case 4:
					Expect(err).To(HaveOccurred())
				}
			})
		}
	})
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
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to float64", func() {
				var dst float64
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int8", func() {
				var dst int8
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int16", func() {
				var dst int16
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int32", func() {
				var dst int32
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int64", func() {
				var dst int64
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to int", func() {
				var dst int
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint8", func() {
				var dst uint8
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint16", func() {
				var dst uint16
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint32", func() {
				var dst uint32
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint64", func() {
				var dst uint64
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
			It("unmarshal correctly to uint", func() {
				var dst uint
				err := m.UnmarshalArgument(src, &dst)
				got := dst
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(data, &dst)).NotTo(HaveOccurred())
				Expect(got).To(BeEquivalentTo(dst))
			})
		}
	})
})
