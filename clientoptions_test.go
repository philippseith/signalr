package signalr

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client options", func() {

	Describe("WithConnection and WithAutoReconnect option", func() {
		Context("none of them is given", func() {
			It("NewClient should fail", func() {
				_, err := NewClient(context.TODO())
				Expect(err).To(HaveOccurred())
			}, 3.0)
		})
		Context("both are given", func() {
			It("NewClient should fail", func() {
				conn := NewNetConnection(context.TODO(), nil)
				_, err := NewClient(context.TODO(), WithConnection(conn), WithAutoReconnect(func() (Connection, error) {
					return conn, nil
				}))
				Expect(err).To(HaveOccurred())
			}, 3.0)
		})
		Context("only WithConnection of is given", func() {
			It("NewClient should not fail", func() {
				conn := NewNetConnection(context.TODO(), nil)
				_, err := NewClient(context.TODO(), WithConnection(conn))
				Expect(err).NotTo(HaveOccurred())
			}, 3.0)
		})
		Context("only WithAutoReconnect of is given", func() {
			It("NewClient should not fail", func() {
				conn := NewNetConnection(context.TODO(), nil)
				_, err := NewClient(context.TODO(), WithAutoReconnect(func() (Connection, error) {
					return conn, nil
				}))
				Expect(err).NotTo(HaveOccurred())
			}, 3.0)
		})

	})

})
