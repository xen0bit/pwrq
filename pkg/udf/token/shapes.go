package token

import "github.com/xen0bit/pwrq/pkg/core/shape"

// JWTShape is a decoded JSON Web Token.
//
// It is Derived rather than Plain because only the top level is the cmdlet's:
// header and payload are objects whose keys are the token's claims, which
// differ from one issuer to the next. Declaring the three outer keys and
// saying where the inner ones come from is the whole truth; declaring a claim
// list would be a guess about someone else's token.
var JWTShape = shape.Derived(
	"the three JWT segments; header and payload hold whatever claims the token carries",
	shape.Prop("header", shape.Object, "decoded header, typically {alg, typ}"),
	shape.Prop("payload", shape.Object, "decoded claims, as the issuer wrote them"),
	shape.Prop("signature", shape.String, "the signature segment, still base64url-encoded and NOT verified"),
)
