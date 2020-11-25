package signalr

import (
	"bytes"
	"fmt"
	"github.com/dave/jennifer/jen"
	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"testing"
)

var _ = Describe("Protocol", func() {
	buf := bytes.Buffer{}
	Context("InvocationMessage jsonHubProtocol", func() {
		p := jsonHubProtocol{}
		p.setDebugLogger(log.NewLogfmtLogger(os.Stderr))
		It("be equal after roundtrip", func() {
			want := invocationMessage{
				Type:         1,
				Target:       "A",
				InvocationID: "B",
				Arguments:    make([]interface{}, 0),
				StreamIds:    nil,
			}
			Expect(p.WriteMessage(want, &buf)).NotTo(HaveOccurred())
			var remainBuf bytes.Buffer
			got, err := p.ParseMessages(&buf, &remainBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(got)).To(Equal(1))
			Expect(got[0]).To(BeEquivalentTo(want))
		})
	})
	Context("InvocationMessage messagePackHubProtocol", func() {
		p := &messagePackHubProtocol{}
		p.setDebugLogger(log.NewLogfmtLogger(os.Stderr))
		It("be equal after roundtrip", func(done Done) {
			want := invocationMessage{
				Type:         1,
				Target:       "A",
				InvocationID: "B",
				Arguments:    make([]interface{}, 0),
				StreamIds:    nil,
			}
			err := p.WriteMessage(want, &buf)
			Expect(err).NotTo(HaveOccurred())
			var remainBuf bytes.Buffer
			got, err := p.ParseMessages(&buf, &remainBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(got)).To(Equal(1))
			Expect(got[0]).To(BeEquivalentTo(want))
			close(done)
		})
	})
	for _, p := range []hubProtocol{
		&jsonHubProtocol{},
		&messagePackHubProtocol{},
	} {
		p.setDebugLogger(log.NewLogfmtLogger(os.Stderr))
		Describe(fmt.Sprintf("%T: WriteMessage/ParseMessages roundtrip", p), func() {
			Context("StreamItemMessage", func() {
				It("be equal after roundtrip", func(done Done) {
					for _, want := range []streamItemMessage{
						//{Type: 2, InvocationID: "", Item: nil},
						{Type: 2, InvocationID: "1", Item: "3"},
						{Type: 2, InvocationID: "1", Item: 3},
						{Type: 2, InvocationID: "1", Item: uint(3)},
						{Type: 2, InvocationID: "1", Item: simpleStruct{AsInt: 3, AsString: "3"}},
						{Type: 2, InvocationID: "1", Item: []int64{1, 2, 3}},
						{Type: 2, InvocationID: "1", Item: map[string]int{"1": 4, "2": 5, "3": 6}},
					} {
						Expect(p.WriteMessage(want, &buf)).NotTo(HaveOccurred())
						var remainBuf bytes.Buffer
						got, err := p.ParseMessages(&buf, &remainBuf)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(got)).To(Equal(1))
						Expect(got[0]).To(BeAssignableToTypeOf(streamItemMessage{}))
						gotMsg := got[0].(streamItemMessage)
						Expect(gotMsg.InvocationID).To(Equal(want.InvocationID))
						t := reflect.TypeOf(want.Item)
						value := reflect.New(t)
						_ = p.UnmarshalArgument(gotMsg.Item, value.Interface())
						Expect(reflect.Indirect(value).Interface()).To(Equal(want.Item))
					}
					close(done)
				})
			})
		})
	}
})

func TestDevParse(t *testing.T) {
	if err := devParse(); err != nil {
		t.Error(err)
	}
}

type simpleStruct struct {
	AsInt    int
	AsString string
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
