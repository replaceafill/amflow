package graph

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"

	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
)

// ignoredVertices includes elements that we don't want to include because they
// slow down rendering times significantly.
var ignoredVertices = map[string]string{
	"61c316a6-0a50-4f65-8767-1f44b1eeb6dd": "Link - Email fail report",
	"7d728c39-395f-4892-8193-92f086c0546f": "Link - Email fail report",
	"333532b9-b7c2-4478-9415-28a3056d58df": "Link - Move to the rejected directory",
	"19c94543-14cb-4158-986b-1d2b55723cd8": "Link - Cleanup rejected SIP",
	"1b04ec43-055c-43b7-9543-bd03c6a778ba": "Chain - Reject transfer",
	"e780473a-0c10-431f-bab6-5d7238b2b70b": "(descendant)",
	"377f8ebb-7989-4a68-9361-658079ff8138": "(descendant)",
	"b2ef06b9-bca4-49da-bc5c-866d7b3c4bb1": "(descendant)",
	"828528c2-2eb9-4514-b5ca-dfd1f7cb5b8c": "(descendant)",
	"3467d003-1603-49e3-b085-e58aa693afed": "(descendant)",
	"ae5cdd0d-2f81-4935-a380-d5c6f1337d93": "(descendant)",
}

func (w Workflow) DOT() ([]byte, error) {
	// Make a copy since we don't want to alter the original graph.
	n := simple.NewDirectedGraph()
	gcopy(n, w)

	// Add initiator
	initiator := &initiatorVertex{node: n.NewNode()}
	n.AddNode(initiator)
	for _, wdir := range w.watchedDirs() {
		if !wdir.isInitiator() {
			continue
		}
		n.SetEdge(n.NewEdge(initiator, wdir))
	}

	// Remove ignored vertices.
	for amid, v := range w.vxByAMID {
		if _, ok := ignoredVertices[amid]; ok {
			n.RemoveNode(v.ID())
		}
	}

	blob, err := dot.Marshal(n, "Archivematica workflow", "", "  ")
	if err != nil {
		return nil, err
	}
	blob = bytes.TrimPrefix(blob, []byte("strict "))
	return blob, nil
}

// SVG returns a blob with the svg element generated by graphviz's dot tool.
func (w *Workflow) SVG() ([]byte, error) {
	blob, err := w.DOT()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = bytes.NewReader(blob)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error: %s - %s", err, b)
	}
	return b, nil
}

// esc escapes strings so they can be used in DOT - see https://git.io/fAAWv.
func esc(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}

// eschtml escapes text in HTML elements.
func eschtml(s string) string {
	if s == "" {
		return "&nbsp;"
	}
	return s
}

// Our graph and vertices implement encode.Node and dot.Attributers so the DOT
// encoders can reach our requirements.

func (v VertexLink) DOTID() string {
	return strconv.Itoa(int(v.ID()))
}

var infoLinks = map[string]string{
	"linkTaskManagerDirectories": "Run the command once.",
	"linkTaskManagerFiles":       "Run the command once for each file.",
}

func (v VertexLink) Attributes() []encoding.Attribute {
	captionbgcolor := "gray"
	execute := v.src.Config.Execute
	if v.src.Config.Model != "StandardTaskConfig" {
		captionbgcolor = "aliceblue"
		execute = "N/A"
	}
	info := "N/A"
	if ret, ok := infoLinks[v.src.Config.Manager]; ok {
		info = ret
	}
	color := "gray"
	if v.IsHighlighted() {
		color = "pink"
	}
	return []encoding.Attribute{
		{Key: "shape", Value: "box"},
		{Key: "color", Value: color},
		{Key: "margin", Value: "0"},
		{Key: "label", Value: fmt.Sprintf(`<
<table border="0" cellborder="1" cellspacing="0">
	<tr><td colspan="2" bgcolor="%s" width="500"><font color="black"><b>%s</b></font></td></tr>
	<tr><td align="left">Type</td><td align="left">Link</td></tr>
	<tr><td align="left">ID</td><td align="left">%s</td></tr>
	<tr><td align="left">Group</td><td align="left">%s</td></tr>
	<tr><td align="left">Manager</td><td align="left">%s</td></tr>
	<tr><td align="left">Model</td><td align="left">%s</td></tr>
	<tr><td align="left">Execute</td><td align="left">%s</td></tr>
	<tr><td align="left">Information</td><td align="left">%s</td></tr>
</table>>`, captionbgcolor,
			eschtml(v.src.Description["en"]),
			eschtml(v.AMID()),
			eschtml(v.src.Group["en"]),
			eschtml(v.src.Config.Manager),
			eschtml(v.src.Config.Model),
			eschtml(execute),
			eschtml(info),
		)},
	}
}

var _ dot.Node = VertexLink{}
var _ encoding.Attributer = VertexLink{}

func (v VertexChainLink) DOTID() string {
	return strconv.Itoa(int(v.ID()))
}

func (v VertexChainLink) Attributes() []encoding.Attribute {
	return []encoding.Attribute{
		{Key: "shape", Value: "box"},
		{Key: "color", Value: "black"},
		{Key: "margin", Value: "0"},
		{Key: "label", Value: fmt.Sprintf(`<
<table border="0" cellborder="1" cellspacing="0">
	<tr><td colspan="2" bgcolor="orange" width="500"><font color="black"><b>%s</b></font></td></tr>
	<tr><td align="left">Type</td><td align="left">Chain</td></tr>
	<tr><td align="left">ID</td><td align="left">%s</td></tr>
</table>>`,
			eschtml(v.src.Description["en"]),
			eschtml(v.AMID()),
		)},
	}
}

var _ dot.Node = VertexChainLink{}
var _ encoding.Attributer = VertexChainLink{}

func (v VertexWatcheDir) DOTID() string {
	return strconv.Itoa(int(v.ID()))
}

func (v VertexWatcheDir) Attributes() []encoding.Attribute {
	attrs := []encoding.Attribute{
		{Key: "label", Value: v.AMID()},
		{Key: "shape", Value: "diamond"},
		{Key: "style", Value: "filled"},
		{Key: "color", Value: "black"},
		{Key: "margin", Value: "0.2"},
	}
	fillcolor := "yellow"
	if v.isInitiator() {
		fillcolor = "green"
	}
	attrs = append(attrs, encoding.Attribute{Key: "fillcolor", Value: fillcolor})
	return attrs
}

var _ dot.Node = VertexWatcheDir{}
var _ encoding.Attributer = VertexWatcheDir{}

type attributer []encoding.Attribute

func (a attributer) Attributes() []encoding.Attribute {
	return a
}

func (w *Workflow) DOTAttributers() (graph, node, edge encoding.Attributer) {
	graph = attributer([]encoding.Attribute{
		{Key: "rankdir", Value: "TB"},
		{Key: "labelloc", Value: "Workflow"},
	})
	node = attributer([]encoding.Attribute{})
	edge = attributer([]encoding.Attribute{})
	return
}
