package signalr

import (
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Streamer", func() {
	Context("Stop with maxCancels set", func() {
		It("should not add entries beyond maxCancels", func() {
			s := &streamer{maxCancels: 2}

			s.Stop("id1")
			s.Stop("id2")
			s.Stop("id3") // should be silently dropped

			Expect(atomic.LoadInt64(&s.cancelCount)).To(Equal(int64(2)))

			present := 0
			s.cancels.Range(func(_, _ interface{}) bool { present++; return true })
			Expect(present).To(Equal(2))
		})

		It("should accept duplicate IDs without double-counting", func() {
			s := &streamer{maxCancels: 2}

			s.Stop("id1")
			s.Stop("id1") // duplicate — LoadOrStore skips the increment
			s.Stop("id2")

			Expect(atomic.LoadInt64(&s.cancelCount)).To(Equal(int64(2)))
		})
	})
})
