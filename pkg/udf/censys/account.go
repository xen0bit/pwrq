package censys

import (
	"fmt"

	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// RegisterGetContext registers get_censys_context, the answer to "why is this
// failing", in the spirit of cencli's `auth status` and `config`.
//
// It reports where the credentials were resolved from and never the token
// itself: a query's output ends up in logs and terminal scrollback, so a
// cmdlet that printed a personal access token would be a way to leak one.
func RegisterGetContext() gojq.CompilerOption {
	const op = "get_censys_context"
	return gojq.WithFunction(op, 0, 1, func(v any, args []any) any {
		var rawOpts map[string]any
		if len(args) == 1 {
			var err error
			if rawOpts, err = optionsArg(op, args[0]); err != nil {
				return err
			}
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return err
		}

		conn, err := resolveConnection(op, connOpts)
		if err != nil {
			return err
		}

		server := conn.ServerURL
		if server == "" {
			server = defaultServerURL()
		}

		context := map[string]any{
			"HasToken":       conn.Token != "",
			"TokenSource":    conn.tokenSource,
			"OrganizationId": conn.OrganizationID,
			"OrgIdSource":    conn.orgSource,
			"ServerUrl":      server,
			"TimeoutSeconds": int(defaultTimeout.Seconds()),

			psobject.PSTypeNameKey: "Censys.Platform.Context",
		}
		if conn.Timeout > 0 {
			context["TimeoutSeconds"] = conn.Timeout
		}
		if conn.Token == "" {
			context["TokenSource"] = ""
		}
		if conn.OrganizationID == "" {
			context["OrgIdSource"] = ""
		}
		return context
	})
}

// defaultServerURL reports the base URL the SDK would use unconfigured.
func defaultServerURL() string {
	if len(censysServerList) == 0 {
		return ""
	}
	return censysServerList[0]
}

// RegisterGetOrganization registers get_censys_organization, cencli's
// `org details`.
func RegisterGetOrganization() gojq.CompilerOption {
	const op = "get_censys_organization"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		orgID, rawOpts, err := optionalIdentifier(op, "OrganizationId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			IncludeMemberCounts bool `param:"IncludeMemberCounts"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}

		sdk, conn, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		// This endpoint puts the organization in the path, so unlike the data
		// endpoints there is no free-wallet fallback to leave it out for.
		if orgID == "" {
			if err := conn.requireOrg(op); err != nil {
				return err
			}
			orgID = conn.OrganizationID
		}

		res, err := sdk.AccountManagement.GetOrganizationDetails(ctx, operations.V3AccountmanagementOrgDetailsRequest{
			OrganizationID:      orgID,
			IncludeMemberCounts: &opts.IncludeMemberCounts,
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeOrganizationDetails == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Organization", res.ResponseEnvelopeOrganizationDetails.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterGetCredits registers get_censys_credits, cencli's `credits` and
// `org credits`.
//
// Which of the two you get follows the credentials: with an organization
// configured these are the organization's credits, and without one they are
// the free account's, exactly as the balance being spent by every other cmdlet
// in this package.
func RegisterGetCredits() gojq.CompilerOption {
	const op = "get_censys_credits"
	return gojq.WithFunction(op, 0, 1, func(v any, args []any) any {
		var rawOpts map[string]any
		if len(args) == 1 {
			var err error
			if rawOpts, err = optionsArg(op, args[0]); err != nil {
				return err
			}
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Scope string `param:"Scope"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}

		sdk, conn, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}

		organization := conn.OrganizationID != ""
		switch opts.Scope {
		case "":
		case "user":
			organization = false
		case "organization":
			organization = true
			if err := conn.requireOrg(op); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: Scope must be user or organization, got %q", op, opts.Scope)
		}

		if organization {
			res, err := sdk.AccountManagement.GetOrganizationCredits(ctx, operations.V3AccountmanagementOrgCreditsRequest{
				OrganizationID: conn.OrganizationID,
			})
			if err != nil {
				return apiError(op, err)
			}
			if res.ResponseEnvelopeOrganizationCredits == nil {
				return fmt.Errorf("%s: the API returned an empty response", op)
			}
			obj, err := object(op, "Censys.Platform.Credits", res.ResponseEnvelopeOrganizationCredits.Result)
			if err != nil {
				return err
			}
			return obj
		}

		res, err := sdk.AccountManagement.GetUserCredits(ctx)
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeUserCredits == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Credits", res.ResponseEnvelopeUserCredits.Result)
		if err != nil {
			return err
		}
		return obj
	})
}
