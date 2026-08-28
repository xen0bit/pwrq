package http

import "github.com/xen0bit/pwrq/pkg/core/shape"

// ResponseShape is what http returns.
//
// It used to share one borrowed .NET name with invoke_web_request, which
// reports a different set of properties for the same idea - Status against
// StatusText, RequestMethod against Method, RequestUri against Url. Two
// property sets under one name is what makes a type name useless as a key, so
// the two now have one each: this is Pwrq.Http.Response, and the cmdlet that
// mirrors PowerShell's property names is Pwrq.Web.Response.
var ResponseShape = shape.Fixed("Pwrq.Http.Response",
	shape.Prop("StatusCode", shape.Number, "HTTP status code"),
	shape.Prop("StatusText", shape.String, "status line, such as 200 OK"),
	shape.Prop("Content", shape.String, "response body; a byte string, so use utf8bytelength to measure it"),
	shape.Prop("ContentLength", shape.Number, "length of the body in bytes"),
	shape.Prop("Headers", shape.Object, "response headers, one key per header"),
	shape.Prop("Method", shape.String, "method that was sent"),
	shape.Prop("Url", shape.String, "URL that was requested"),
	shape.OptProp("RequestBody", shape.String, "the body that was sent; absent when there was none"),
	shape.OptProp("RequestContentLength", shape.Number, "its length in bytes; absent when there was no body"),
)
