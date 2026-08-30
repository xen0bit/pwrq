package xml

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ParsedXML is what xml_parse emits: the element's own name and body, plus its
// attributes when it has any.
//
// The keys are pwrq's, not the document's - a child element does not become a
// key, it stays inside _content as text - so this is a Plain shape rather than
// a Derived one. Saying so matters because the underscore names are not
// guessable, and a caller who assumes xml_parse gives them the document's own
// element names writes a selector that silently matches nothing.
var ParsedXML = shape.Plain(
	shape.Prop("_tag", shape.String, "the element's name"),
	shape.Prop("_content", shape.String, "the element's body, with any child elements left as XML text"),
	shape.OptProp("_attrs", shape.Object, "the element's attributes, one key per attribute; absent when it has none"),
)
