package yaml

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ParsedYAML is whatever the document held. A YAML document is not necessarily
// a mapping - a bare scalar and a sequence are both valid - so this is a
// Derived shape rather than a Fixed one even though the cmdlet is a parser.
var ParsedYAML = shape.Derived("the document's own structure; an object when the document is a mapping, and a scalar or array otherwise")
