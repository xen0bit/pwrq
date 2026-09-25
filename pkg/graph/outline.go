package graph

import "github.com/itchyny/gojq"

// OutlineNode is one box of a query's diagram: a label, the class that colours
// it, and the boxes drawn inside it.
type OutlineNode struct {
	Label    string
	Class    string
	Children []*OutlineNode
}

// Outline is a query's diagram as a tree rather than a picture, for a host
// that cannot draw one - a terminal. It is recorded while the D2 script is
// written, by the same builder, so the outline and the diagram cannot
// disagree about what a query contains or what colour each part is.
//
// The top level is in the order the diagram reads: the definitions first,
// then each stage of the pipeline. The Start and End markers are left out;
// in a list they say nothing the list's own beginning and end do not.
func Outline(query *gojq.Query, opts RenderOptions) []*OutlineNode {
	root := &OutlineNode{}
	b := &builder{opts: opts, outline: &outliner{stack: []*OutlineNode{root}}}
	b.render(query)
	return root.Children
}

// outliner builds the tree. Its methods are no-ops on a nil receiver, so the
// builder can call them unconditionally when only a script is wanted.
type outliner struct {
	stack []*OutlineNode
}

func (o *outliner) leaf(label, class string) {
	if o == nil || class == ClassTerminal {
		return
	}
	o.top().Children = append(o.top().Children, &OutlineNode{Label: label, Class: class})
}

func (o *outliner) open(label, class string) {
	if o == nil {
		return
	}
	node := &OutlineNode{Label: label, Class: class}
	o.top().Children = append(o.top().Children, node)
	o.stack = append(o.stack, node)
}

func (o *outliner) close() {
	if o == nil || len(o.stack) == 1 {
		return
	}
	o.stack = o.stack[:len(o.stack)-1]
}

func (o *outliner) top() *OutlineNode { return o.stack[len(o.stack)-1] }
