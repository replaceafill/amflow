package graph

import (
	"fmt"
	"strings"

	"github.com/gobuffalo/packr/v2"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"

	amjson "github.com/sevein/amflow/internal/graph/encoding"
)

var WorkflowSchemaBox = packr.New("workflow", "./schema")

// Workflow is a sequence of operations in Archivematica.
//
// It is modeled as a simple directed graph.
type Workflow struct {
	// Underlying directed graph.
	graph *simple.DirectedGraph

	// Internal mappings for convenience.
	vxByID   map[int64]Vertex
	vxByAMID map[string]Vertex
}

// New returns a Workflow.
func New(data *amjson.WorkflowData) *Workflow {
	w := &Workflow{
		graph:    simple.NewDirectedGraph(),
		vxByID:   map[int64]Vertex{},
		vxByAMID: map[string]Vertex{},
	}
	if data != nil {
		w.load(data)
	}
	return w
}

// AddVertex adds a new vertex to the workflow.
func (w *Workflow) addVertex(v amjson.Vertex) Vertex {
	var vertex Vertex
	switch v := v.(type) {
	case *amjson.Chain:
		vertex = &VertexChainLink{
			v:   w.graph.NewNode(),
			src: v,
		}
	case *amjson.Link:
		vertex = &VertexLink{
			v:   w.graph.NewNode(),
			src: v,
		}
	case *amjson.WatchedDirectory:
		vertex = &VertexWatcheDir{
			v:   w.graph.NewNode(),
			src: v,
		}
	}
	w.graph.AddNode(vertex)
	w.vxByID[vertex.ID()] = vertex
	w.vxByAMID[vertex.AMID()] = vertex
	return vertex
}

// Vertex returns a workflow vertex given its AMD.
func (w Workflow) VertexByAMID(amid string) Vertex {
	v, ok := w.vxByAMID[amid]
	if !ok {
		return nil
	}
	return v
}

// Vertex returns a workflow vertex given its ID.
func (w Workflow) VertexByID(id int64) Vertex {
	v, ok := w.vxByID[id]
	if !ok {
		return nil
	}
	return v
}

// hasMultipleComponents determines if every vertex is reachable from every
// other vertex. Currently, Archivematica workflows are not expected to have
// more than one component (subgraph). This is a property observed in the
// existing workflow dataset but it may stop being that way in the future.
func (w Workflow) hasMultipleComponents() bool {
	cc := topo.ConnectedComponents(graph.Undirect{G: w.graph})
	return len(cc) > 1
}

func (w Workflow) watchedDirs() []*VertexWatcheDir {
	ret := []*VertexWatcheDir{}
	for _, v := range w.vxByID {
		vwd, ok := v.(*VertexWatcheDir)
		if ok {
			ret = append(ret, vwd)
		}
	}
	return ret
}

// load workflow data. It includes vertices and edges. The latter are mostly
// explicit in the workflow data, excepting move filesystem operations.
func (w *Workflow) load(data *amjson.WorkflowData) {
	// Links.
	_lns := make(map[string]*VertexLink)
	for id, item := range data.Links {
		_lns[id] = w.addVertex(item).(*VertexLink)
	}

	// Chain links.
	_chs := make(map[string]*VertexChainLink)
	for id, item := range data.Chains {
		vertexSrc := w.addVertex(item).(*VertexChainLink)
		_chs[id] = vertexSrc
		if vertexDst, ok := _lns[item.LinkID]; ok {
			w.graph.SetEdge(w.graph.NewEdge(vertexSrc, vertexDst))
		}
	}

	// Watched directories.
	_wds := make(map[string]*VertexWatcheDir)
	for _, item := range data.WatchedDirectories {
		vertexSrc := w.addVertex(item).(*VertexWatcheDir)
		_wds[item.Path] = vertexSrc
		if vertexDst, ok := _chs[item.ChainID]; ok {
			w.graph.SetEdge(w.graph.NewEdge(vertexSrc, vertexDst))
		}
	}

	// Build a map of variables defined in TaskConfigSetUnitVariable links
	// and their respective links. This is going to be useful later to connect
	// pull links.
	_vars := map[string][]*VertexLink{}
	for _, node := range _lns {
		if node.src.Config.Model == "TaskConfigSetUnitVariable" {
			if match, ok := _lns[node.src.Config.ChainID]; ok {
				_vars[node.src.Config.Variable] = append(_vars[node.src.Config.Variable], match)
			}
		}
	}

	// Another pass to connect links.
	for _, vertexSrc := range _lns {
		// Connect to other links based on the fallback defined.
		if vertexSrc.src.FallbackLinkID != "" {
			if vertexDst, ok := _lns[vertexSrc.src.FallbackLinkID]; ok {
				w.graph.SetEdge(newDefaultFallbackEdge(vertexSrc, vertexDst, vertexSrc.src.FallbackJobStatus))
			}
		}

		// Connect to other links based on the exit codes.
		for code, ec := range vertexSrc.src.ExitCodes {
			if ec.LinkID == "" {
				continue
			}
			if vertexDst, ok := _lns[ec.LinkID]; ok {
				var hasFallback bool
				if e := w.graph.Edge(vertexSrc.ID(), vertexDst.ID()); e != nil {
					hasFallback = true
				}
				w.graph.SetEdge(newExitCodeEdge(vertexSrc, vertexDst, code, ec.JobStatus, hasFallback))
			}
		}

		switch {
		case vertexSrc.src.Config.Model == "MicroServiceChainChoice" && len(vertexSrc.src.Config.Choices) > 0:
			{
				for _, id := range vertexSrc.src.Config.Choices {
					if vertexDst, ok := _chs[id]; ok {
						w.graph.SetEdge(newChainChoiceEdge(vertexSrc, vertexDst))
					}
				}
			}
		case vertexSrc.src.Config.Manager == "linkTaskManagerUnitVariableLinkPull":
			{
				if values, ok := _vars[vertexSrc.src.Config.Variable]; ok {
					for _, vertexDst := range values {
						w.graph.SetEdge(w.graph.NewEdge(vertexSrc, vertexDst))
					}
				}
				if vertexSrc.src.Config.ChainID != "" {
					if vertexDst, ok := _lns[vertexSrc.src.Config.ChainID]; ok {
						w.graph.SetEdge(w.graph.NewEdge(vertexSrc, vertexDst))
					}
				}
			}
		// This section below declares edges for associations that are a result
		// of filesystem moving operations that MCPServer identifies by watching
		// directories. We've found this mechanism to be undesirable and it will
		// probably change soon.
		case vertexSrc.src.Config.Manager == "linkTaskManagerDirectories":
			{
				if strings.HasPrefix(vertexSrc.src.Config.Execute, "move") {
					args := vertexSrc.src.Config.Arguments
					for path, vertexDst := range _wds {
						substr1 := fmt.Sprintf("%%watchedDirectories%s", path)
						substr2 := fmt.Sprintf("%%watchDirectoryPath%%%s", path[1:])
						if strings.Contains(args, substr1) || strings.Contains(args, substr2) {
							w.graph.SetEdge(newVirtualMovingDirBridge(vertexSrc, vertexDst))
						}
					}
				} else if vertexSrc.src.Description["en"] == "Create SIP from transfer objects" || vertexSrc.src.Description["en"] == "Create SIPs from TRIM transfer containers" {
					w.graph.SetEdge(newVirtualMovingDirBridge(vertexSrc, w.VertexByAMID("/system/autoProcessSIP")))
				}
			}
		}
	}
}

// Implement graph.Graph.
func (w Workflow) Node(id int64) graph.Node           { return w.graph.Node(id) }
func (w Workflow) Nodes() graph.Nodes                 { return w.graph.Nodes() }
func (w Workflow) From(id int64) graph.Nodes          { return w.graph.From(id) }
func (w Workflow) HasEdgeBetween(xid, yid int64) bool { return w.graph.HasEdgeBetween(xid, yid) }
func (w Workflow) Edge(uid, vid int64) graph.Edge     { return w.graph.Edge(uid, vid) }

var _ graph.Graph = Workflow{}

// Implement graph.Directed.
func (w Workflow) HasEdgeFromTo(uid, vid int64) bool { return w.graph.HasEdgeFromTo(uid, vid) }
func (w Workflow) To(id int64) graph.Nodes           { return w.graph.To(id) }

var _ graph.Directed = Workflow{}
