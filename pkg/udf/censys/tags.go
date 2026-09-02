package censys

import (
	"fmt"

	"github.com/censys/censys-sdk-go/models/components"
	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterGetTag registers get_censys_tag, cencli's `tags get` and
// `tags list`: one tag when given an ID or name, otherwise all of them.
func RegisterGetTag() gojq.CompilerOption {
	const op = "get_censys_tag"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		tagID, rawOpts, err := optionalIdentifier(op, "TagId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Name      string `param:"Name"`
			Privacy   string `param:"Privacy"`
			CreatedBy string `param:"CreatedBy"`
			OrderBy   string `param:"OrderBy"`
			PageSize  int    `param:"PageSize"`
			PageToken string `param:"PageToken"`
			Pages     int    `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		if tagID != "" {
			res, err := sdk.TagsAndComments.GetTag(ctx, operations.V3TagsGetTagRequest{TagID: tagID})
			if err != nil {
				return gojq.NewIter(apiError(op, err))
			}
			if res.ResponseEnvelopeTag == nil {
				return gojq.NewIter(fmt.Errorf("%s: the API returned an empty response", op))
			}
			obj, err := object(op, "Censys.Platform.Tag", res.ResponseEnvelopeTag.Result)
			if err != nil {
				return gojq.NewIter(err)
			}
			return gojq.NewIter[any](obj)
		}

		var privacy *operations.Privacy
		if opts.Privacy != "" {
			p := operations.Privacy(opts.Privacy)
			privacy = &p
		}
		var orderBy *operations.V3TagsListTagsQueryParamOrderBy
		if opts.OrderBy != "" {
			o := operations.V3TagsListTagsQueryParamOrderBy(opts.OrderBy)
			orderBy = &o
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.TagsAndComments.ListTags(ctx, operations.V3TagsListTagsRequest{
				Name:      optString(opts.Name),
				CreatedBy: optString(opts.CreatedBy),
				Privacy:   privacy,
				OrderBy:   orderBy,
				PageSize:  optInt(opts.PageSize),
				PageToken: optString(token),
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeTagsList == nil || res.ResponseEnvelopeTagsList.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeTagsList.Result
			batch, err := items(op, "Censys.Platform.Tag", result.Tags)
			if err != nil {
				return nil, "", err
			}
			return batch, deref(result.NextPageToken), nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}

// RegisterNewTag registers new_censys_tag, cencli's `tags create`.
//
// Privacy is a required field of the API's body with no default, so an
// unspecified tag is created shared — visible to the whole organization, which
// is what a tag is usually for.
//
// A write: registered only when EnvWrite asks for it.
func RegisterNewTag() gojq.CompilerOption {
	const op = "new_censys_tag"
	return common.WithFunction(op, 0, 1, func(v any, args []any) any {
		props, err := propertyArgs(op, v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(props)

		var body struct {
			Name        string `param:"Name"`
			Description string `param:"Description"`
			Privacy     string `param:"Privacy"`
		}
		if err := bindOptions(op, rest, &body); err != nil {
			return err
		}
		if body.Name == "" {
			return fmt.Errorf("%s: Name is required", op)
		}
		if body.Privacy == "" {
			body.Privacy = string(components.CreateTagInputBodyPrivacyShared)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.TagsAndComments.CreateTag(ctx, operations.V3TagsCreateTagRequest{
			CreateTagInputBody: components.CreateTagInputBody{
				Name:        body.Name,
				Description: optString(body.Description),
				Privacy:     components.CreateTagInputBodyPrivacy(body.Privacy),
			},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeTag == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Tag", res.ResponseEnvelopeTag.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterSetTag registers set_censys_tag, cencli's `tags update`. Every field
// of the body is optional, so this one really is a patch.
//
// A write: registered only when EnvWrite asks for it.
func RegisterSetTag() gojq.CompilerOption {
	const op = "set_censys_tag"
	return common.WithFunction(op, 1, 2, func(v any, args []any) any {
		tagID, props, err := identifier(op, "TagId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(props)

		var body struct {
			Name        string `param:"Name"`
			Description string `param:"Description"`
			Privacy     string `param:"Privacy"`
		}
		if err := bindOptions(op, rest, &body); err != nil {
			return err
		}
		if body.Name == "" && body.Description == "" && body.Privacy == "" {
			return fmt.Errorf("%s: give at least one of Name, Description or Privacy to change", op)
		}

		var privacy *components.UpdateTagInputBodyPrivacy
		if body.Privacy != "" {
			p := components.UpdateTagInputBodyPrivacy(body.Privacy)
			privacy = &p
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.TagsAndComments.UpdateTag(ctx, operations.V3TagsUpdateTagRequest{
			TagID: tagID,
			UpdateTagInputBody: components.UpdateTagInputBody{
				Name:        optString(body.Name),
				Description: optString(body.Description),
				Privacy:     privacy,
			},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeTag == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Tag", res.ResponseEnvelopeTag.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterRemoveTag registers remove_censys_tag, cencli's `tags delete`.
//
// A write: registered only when EnvWrite asks for it.
func RegisterRemoveTag() gojq.CompilerOption {
	const op = "remove_censys_tag"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		tagID, rawOpts, err := identifier(op, "TagId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		if _, err := sdk.TagsAndComments.DeleteTag(ctx, operations.V3TagsDeleteTagRequest{TagID: tagID}); err != nil {
			return apiError(op, err)
		}
		return tagID
	})
}

// RegisterGetTagAssignment registers get_censys_tag_assignment, cencli's
// `tags assignments`: what a tag is currently attached to.
func RegisterGetTagAssignment() gojq.CompilerOption {
	const op = "get_censys_tag_assignment"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		tagID, rawOpts, err := identifier(op, "TagId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			AssetId       string `param:"AssetId"`
			AssetType     string `param:"AssetType"`
			CreatedBefore string `param:"CreatedBefore"`
			CreatedAfter  string `param:"CreatedAfter"`
			CreatedBy     string `param:"CreatedBy"`
			OrderBy       string `param:"OrderBy"`
			PageSize      int    `param:"PageSize"`
			PageToken     string `param:"PageToken"`
			Pages         int    `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}
		createdBefore, err := parseTime(op, "CreatedBefore", opts.CreatedBefore)
		if err != nil {
			return gojq.NewIter(err)
		}
		createdAfter, err := parseTime(op, "CreatedAfter", opts.CreatedAfter)
		if err != nil {
			return gojq.NewIter(err)
		}

		var assetType *operations.AssetType
		if opts.AssetType != "" {
			a := operations.AssetType(opts.AssetType)
			assetType = &a
		}
		var orderBy *operations.V3TagsListAssignmentsQueryParamOrderBy
		if opts.OrderBy != "" {
			o := operations.V3TagsListAssignmentsQueryParamOrderBy(opts.OrderBy)
			orderBy = &o
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.TagsAndComments.ListTagAssignments(ctx, operations.V3TagsListAssignmentsRequest{
				TagID:         tagID,
				AssetID:       optString(opts.AssetId),
				AssetType:     assetType,
				CreatedBefore: createdBefore,
				CreatedAfter:  createdAfter,
				CreatedBy:     optString(opts.CreatedBy),
				OrderBy:       orderBy,
				PageSize:      optInt(opts.PageSize),
				PageToken:     optString(token),
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeTagAssignmentsList == nil || res.ResponseEnvelopeTagAssignmentsList.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeTagAssignmentsList.Result
			batch, err := items(op, "Censys.Platform.TagAssignment", result.Assignments)
			if err != nil {
				return nil, "", err
			}
			return batch, deref(result.NextPageToken), nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}

// RegisterAddTagAssignment registers add_censys_tag_assignment, cencli's
// `tags assign`.
//
// The asset is the half that comes from the pipeline, because tagging a set of
// hosts found by a search is the reason this cmdlet exists:
//
//	search_censys("...") | .host.ip | add_censys_tag_assignment("compromised")
//
// A write: registered only when EnvWrite asks for it.
func RegisterAddTagAssignment() gojq.CompilerOption {
	const op = "add_censys_tag_assignment"
	return common.WithFunction(op, 1, 3, func(v any, args []any) any {
		tagID, assetID, rawOpts, err := pairArgs(op, "TagId", "AssetId", 1, v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.TagsAndComments.CreateTagAssignment(ctx, operations.V3TagsCreateAssignmentRequest{
			TagID: tagID,
			CreateTagAssignmentInputBody: components.CreateTagAssignmentInputBody{
				AssetID: assetID,
			},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeTagAssignment == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.TagAssignment", res.ResponseEnvelopeTagAssignment.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterRemoveTagAssignment registers remove_censys_tag_assignment, cencli's
// `tags unassign`. It takes the assignment's own ID, which
// get_censys_tag_assignment reports — not the asset ID.
//
// A write: registered only when EnvWrite asks for it.
func RegisterRemoveTagAssignment() gojq.CompilerOption {
	const op = "remove_censys_tag_assignment"
	return common.WithFunction(op, 1, 3, func(v any, args []any) any {
		tagID, assignmentID, rawOpts, err := pairArgs(op, "TagId", "AssignmentId", 1, v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		if _, err := sdk.TagsAndComments.DeleteTagAssignment(ctx, operations.V3TagsDeleteAssignmentRequest{
			TagID:        tagID,
			AssignmentID: assignmentID,
		}); err != nil {
			return apiError(op, err)
		}
		return assignmentID
	})
}
