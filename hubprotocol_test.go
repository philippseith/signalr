package signalr

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/dave/jennifer/jen"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Protocol", func() {
	for _, p := range []hubProtocol{
		&jsonHubProtocol{},
		&messagePackHubProtocol{},
	} {
		protocol := p
		protocol.setDebugLogger(testLogger())
		Describe(fmt.Sprintf("%T: WriteMessage/ParseMessages roundtrip", protocol), func() {
			Context("InvocationMessage", func() {
				for _, a := range [][]interface{}{
					make([]interface{}, 0),
					{1, 2, 3},
					{1, 0xffffff},
					{-5, []int{1000, 2}, simpleStruct{AsInt: 3, AsString: "3"}},
					{[]simpleStruct{
						{AsInt: 3, AsString: "3"},
						{AsInt: 40, AsString: "40"},
					}},
					{map[string]int{"1": 2, "2": 4, "3": 8}},
					{map[int]simpleStruct{1: {AsInt: 1, AsString: "1"}, 2: {AsInt: 2, AsString: "2"}}},
				} {
					arguments := a
					want := invocationMessage{
						Type:         1,
						Target:       "A",
						InvocationID: "B",
						Arguments:    arguments,
						StreamIds:    []string{"C", "D"},
					}
					It(fmt.Sprintf("be equal after roundtrip with arguments %v", arguments), func() {
						buf := bytes.Buffer{}
						Expect(protocol.WriteMessage(want, &buf)).NotTo(HaveOccurred())
						Expect(len(msg)).NotTo(Equal(0))
						var remainBuf bytes.Buffer
						got, err := protocol.ParseMessages(&buf, &remainBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(got)).To(Equal(1))
						Expect(got[0]).To(BeAssignableToTypeOf(invocationMessage{}))
						gotMsg := got[0].(invocationMessage)
						Expect(gotMsg.Target).To(Equal(want.Target))
						Expect(gotMsg.InvocationID).To(Equal(want.InvocationID))
						Expect(gotMsg.StreamIds).To(Equal(want.StreamIds))
						Expect(len(gotMsg.Arguments)).To(Equal(len(want.Arguments)))
						for i, gotArg := range gotMsg.Arguments {
							// We can not directly compare gotArg and want.Arguments[i]
							// because msgpack serializes numbers to the shortest possible type
							t := reflect.TypeOf(want.Arguments[i])
							value := reflect.New(t)
							Expect(protocol.UnmarshalArgument(gotArg, value.Interface())).NotTo(HaveOccurred())
							Expect(reflect.Indirect(value).Interface()).To(Equal(want.Arguments[i]))
						}
					})
				}
			})
			Context("StreamItemMessage", func() {
				for _, w := range []streamItemMessage{
					{Type: 2, InvocationID: "1", Item: "3"},
					{Type: 2, InvocationID: "2", Item: 3},
					{Type: 2, InvocationID: "3", Item: uint(3)},
					{Type: 2, InvocationID: "4", Item: simpleStruct{AsInt: 3, AsString: "3"}},
					{Type: 2, InvocationID: "5", Item: []int64{1, 2, 3}},
					{Type: 2, InvocationID: "6", Item: []int{1, 2, 3}},
					{Type: 2, InvocationID: "7", Item: map[string]int{"1": 4, "2": 5, "3": 6}},
					{Type: 2, InvocationID: "9"},
				} {
					want := w
					It(fmt.Sprintf("should be equal after roundtrip of %#v", want), func(done Done) {
						buf := bytes.Buffer{}
						Expect(protocol.WriteMessage(want, &buf)).NotTo(HaveOccurred())
						var remainBuf bytes.Buffer
						got, err := protocol.ParseMessages(&buf, &remainBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(got)).To(Equal(1))
						Expect(got[0]).To(BeAssignableToTypeOf(streamItemMessage{}))
						gotMsg := got[0].(streamItemMessage)
						Expect(gotMsg.InvocationID).To(Equal(want.InvocationID))
						if want.Item == nil {
							var v interface{}
							Expect(protocol.UnmarshalArgument(gotMsg.Item, &v)).NotTo(HaveOccurred())
							Expect(v).To(BeNil())
						} else {
							// We can not directly compare gotArg and want.Arguments[i]
							// because msgpack serializes numbers to the shortest possible type
							t := reflect.TypeOf(want.Item)
							value := reflect.New(t)
							Expect(protocol.UnmarshalArgument(gotMsg.Item, value.Interface())).NotTo(HaveOccurred())
							Expect(reflect.Indirect(value).Interface()).To(Equal(want.Item))
						}
						close(done)
					})
				}
			})
			Context("CompletionMessage", func() {
				for _, w := range []completionMessage{
					{Type: 3, InvocationID: "1", Result: "3"},
					{Type: 3, InvocationID: "2", Result: 3},
					{Type: 3, InvocationID: "3", Result: uint(3)},
					{Type: 3, InvocationID: "4", Result: simpleStruct{AsInt: 3, AsString: "3"}},
					{Type: 3, InvocationID: "5", Result: []int64{1, 2, 3}},
					{Type: 3, InvocationID: "6", Result: []int{1, 2, 3}},
					{Type: 3, InvocationID: "7", Result: map[string]int{"1": 4, "2": 5, "3": 6}},
					{Type: 3, InvocationID: "8"},
					{Type: 3, InvocationID: "9", Error: "Failed"},
				} {
					want := w
					It(fmt.Sprintf("should be equal after roundtrip of %#v", want), func(done Done) {
						buf := bytes.Buffer{}
						Expect(protocol.WriteMessage(want, &buf)).NotTo(HaveOccurred())
						var remainBuf bytes.Buffer
						got, err := protocol.ParseMessages(&buf, &remainBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(got)).To(Equal(1))
						Expect(got[0]).To(BeAssignableToTypeOf(completionMessage{}))
						gotMsg := got[0].(completionMessage)
						Expect(gotMsg.InvocationID).To(Equal(want.InvocationID))
						if want.Result == nil {
							// Important: In contrast to StreamItemMessage a nil Result is not transmitted
							// So if a stream ends with a nil item,
							// a sender can not send a completionMessage with nil result to transmit this!
							Expect(gotMsg.Result).To(BeNil())
							Expect(gotMsg.Error).To(Equal(want.Error))
						} else {
							// We can not directly compare gotArg and want.Arguments[i]
							// because msgpack serializes numbers to the shortest possible type
							t := reflect.TypeOf(want.Result)
							value := reflect.New(t)
							Expect(protocol.UnmarshalArgument(gotMsg.Result, value.Interface())).NotTo(HaveOccurred())
							Expect(reflect.Indirect(value).Interface()).To(Equal(want.Result))
							Expect(gotMsg.Error).To(Equal(want.Error))
						}
						close(done)
					})
				}
			})
			Context("Multiple messages", func() {
				It("should parse multiple messages sent in one step", func(done Done) {
					buf := bytes.Buffer{}
					streamItem := streamItemMessage{Type: 2, InvocationID: "2", Item: "A"}
					completion := completionMessage{Type: 3, InvocationID: "2", Result: "B"}
					Expect(protocol.WriteMessage(streamItem, &buf)).NotTo(HaveOccurred())
					Expect(protocol.WriteMessage(completion, &buf)).NotTo(HaveOccurred())
					var remainBuf bytes.Buffer
					got, err := protocol.ParseMessages(&buf, &remainBuf)
					Expect(err).NotTo(HaveOccurred())
					Expect(len(got)).To(Equal(2))
					Expect(got[0]).To(BeAssignableToTypeOf(streamItemMessage{}))
					gotStreamItem := got[0].(streamItemMessage)
					var item string
					Expect(protocol.UnmarshalArgument(gotStreamItem.Item, &item)).NotTo(HaveOccurred())
					Expect(item).To(Equal(streamItem.Item))
					Expect(got[1]).To(BeAssignableToTypeOf(completionMessage{}))
					gotCompletion := got[1].(completionMessage)
					var result string
					Expect(protocol.UnmarshalArgument(gotCompletion.Result, &result)).NotTo(HaveOccurred())
					Expect(result).To(Equal(completion.Result))
					close(done)
				})
			})
			Context("Partial messages", func() {
				It("should parse a message sent in two steps", func(done Done) {
					messageBuf := &bytes.Buffer{}
					streamItem := streamItemMessage{Type: 2, InvocationID: "2", Item: "A"}
					Expect(protocol.WriteMessage(streamItem, messageBuf)).NotTo(HaveOccurred())
					reader, writer := io.Pipe()
					var remainBuf bytes.Buffer
					// Store incomplete frame
					go func() {
						defer GinkgoRecover()
						_, err := writer.Write(messageBuf.Bytes()[:messageBuf.Len()-2])
						Expect(err).NotTo(HaveOccurred())
					}()
					up := make(chan struct{}, 1)
					go func() {
						defer GinkgoRecover()
						up <- struct{}{}
						got, err := protocol.ParseMessages(reader, &remainBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(got)).To(Equal(1))
						Expect(got[0]).To(BeAssignableToTypeOf(streamItemMessage{}))
						gotStreamItem := got[0].(streamItemMessage)
						var item string
						Expect(protocol.UnmarshalArgument(gotStreamItem.Item, &item)).NotTo(HaveOccurred())
						Expect(item).To(Equal(streamItem.Item))
						close(done)
					}()
					// Wait for parse to be started
					<-up
					// Let parse hang a while
					<-time.After(time.Millisecond * 200)
					// Write the rest of the frame
					_, err := writer.Write(messageBuf.Bytes()[messageBuf.Len()-2:])
					Expect(err).NotTo(HaveOccurred())
				}, 2.0)
			})
		})
	}
})

func TestDevParse(t *testing.T) {
	if err := devParse(); err != nil {
		t.Error(err)
	}
}

//type simplestStruct struct {
//	AsInt int
//}

type simpleStruct struct {
	AsInt    int    `json:"AI"`
	AsString string `json:"AS"`
}

//type parserHub struct {
//	Hub
//}
//
//func (p *parserHub) Parse(fileName string) []string {
//	return nil
//}

func devParse() error {
	fSet := token.NewFileSet()
	file, err := parser.ParseFile(fSet, "hubprotocol_test.go", nil, parser.AllErrors)
	if err != nil {
		return err
	}
	g := generator{hubs: make(map[string]*hubInfo)}
	ast.Walk(&g, file)
	g.Generate()
	return nil
}

type generator struct {
	//packageName string
	hubs map[string]*hubInfo
}

type hubInfo struct {
	receiver  string
	funcDecls []*ast.FuncDecl
}

func (g *generator) Generate() {
	f := jen.NewFile("t1")
	for hub, hubInfo := range g.hubs {
		g.generateInvokeProtocol(f, "JSON", hub, hubInfo)
		g.generateInvokeProtocol(f, "MessagePack", hub, hubInfo)
	}
	fmt.Printf("%#v", f)
}

func (g *generator) generateInvokeProtocol(f *jen.File, protocol string, hub string, hubInfo *hubInfo) {
	targetCases := make([]jen.Code, 0)
	for _, funcDecl := range hubInfo.funcDecls {
		targetCases = append(targetCases, jen.Case(jen.Lit(funcDecl.Name.Name)).
			Block(
				jen.Return(jen.Id("Invoke"+funcDecl.Name.Name+protocol)).
					Params(
						jen.Id("arguments"),
						jen.Id("streamIds"))))
	}
	f.Func().Params(jen.Id(hubInfo.receiver).Op("*").Id(hub)).Id("Invoke"+protocol).
		Params(
			jen.Id("target").String(),
			jen.Id("arguments").Interface(),
			jen.Id("streamIds").Index().String()).
		Params(jen.Interface(), jen.Error()).
		Block(
			jen.Switch(jen.Id("target")).
				Block(targetCases...),
			jen.Return(jen.Nil(), jen.Qual("errors", "New").
				Params(
					jen.Lit("invalid target ").Op("+").Id("target"))))

}

func (g *generator) Visit(node ast.Node) (w ast.Visitor) {
	if node == nil {
		return nil
	}
	switch value := node.(type) {
	case *ast.TypeSpec:
		if structType, ok := value.Type.(*ast.StructType); ok {
			if len(structType.Fields.List) > 0 {
				if ident, ok := structType.Fields.List[0].Type.(*ast.Ident); ok && ident.Name == "Hub" {
					if _, ok := g.hubs[value.Name.Name]; !ok {
						g.hubs[value.Name.Name] = &hubInfo{
							funcDecls: make([]*ast.FuncDecl, 0),
						}
					}
				}
			}
		}
	case *ast.FuncDecl:
		if value.Recv != nil && len(value.Recv.List) == 1 {
			switch recvType := value.Recv.List[0].Type.(type) {
			case *ast.Ident:
				if hubInfo, ok := g.hubs[recvType.Name]; ok {
					hubInfo.receiver = value.Recv.List[0].Names[0].Name
					hubInfo.funcDecls = append(hubInfo.funcDecls, value)
				}
			case *ast.StarExpr:
				if recvType, ok := recvType.X.(*ast.Ident); ok {
					if hubInfo, ok := g.hubs[recvType.Name]; ok {
						hubInfo.receiver = value.Recv.List[0].Names[0].Name
						hubInfo.funcDecls = append(hubInfo.funcDecls, value)
					}
				}
			}
		}
	}
	return g
}
