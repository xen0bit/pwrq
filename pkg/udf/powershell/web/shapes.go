package web

import "github.com/xen0bit/pwrq/pkg/core/shape"

// WebResponseShape is what invoke_web_request returns.
//
// The property names are PowerShell's, because the cmdlet exists so a query
// written against Invoke-WebRequest keeps working. The type name is pwrq's,
// because the object is pwrq's: nothing behind it is a .NET class. pwrq's
// leaner http cmdlet reports the same idea under Pwrq.Http.Response.
var WebResponseShape = shape.Fixed("Pwrq.Web.Response",
	shape.Prop("StatusCode", shape.Number, "HTTP status code"),
	shape.Prop("Status", shape.String, "status line, such as 200 OK"),
	shape.Prop("Content", shape.String, "response body; a byte string, so use utf8bytelength to measure it"),
	shape.Prop("ContentLength", shape.Number, "length of the body in bytes"),
	shape.Prop("ContentType", shape.String, "the Content-Type header, or \"\""),
	shape.Prop("Headers", shape.Object, "response headers, one key per header"),
	shape.Prop("BaseResponse", shape.Any, "the underlying response, in the form PowerShell exposes"),
	shape.Prop("RequestMethod", shape.String, "method that was sent"),
	shape.Prop("RequestUri", shape.String, "URL that was requested"),
	shape.Prop("ResponseUri", shape.String, "URL that answered, after any redirects"),
	shape.Prop("LastModified", shape.String, "the Last-Modified header as RFC 3339"),
)
